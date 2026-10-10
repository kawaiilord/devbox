package app

import (
	"context"
	"errors"
	"sort"
	"time"
)

var (
	ErrAlreadyCheckedIn   = errors.New("already checked in today")
	ErrInsufficientPoints = errors.New("insufficient points")
	ErrRedemptionDisabled = errors.New("points redemption is disabled")
	ErrInvalidPayment     = errors.New("invalid payment")
)

type memoryPointsAccount struct {
	balance, earned, spent int64
}

func (r *MemoryRepository) ListVIPPlans(_ context.Context, includeDisabled bool) ([]VIPPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []VIPPlan{}
	for _, plan := range r.vipPlans {
		if includeDisabled || plan.Enabled {
			out = append(out, plan)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PriceMinor < out[j].PriceMinor })
	return out, nil
}

func (r *MemoryRepository) GetVIPPlan(_ context.Context, id string) (VIPPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plan, ok := r.vipPlans[id]
	if !ok || !plan.Enabled {
		return VIPPlan{}, ErrNotFound
	}
	return plan, nil
}

func (r *MemoryRepository) CreateOrder(_ context.Context, order Order) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.orders[order.OrderNo]; exists {
		return Order{}, errors.New("order already exists")
	}
	r.orders[order.OrderNo] = order
	return order, nil
}

func (r *MemoryRepository) SetOrderCheckout(_ context.Context, orderNo, checkoutURL string, expiresAt int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	order, ok := r.orders[orderNo]
	if !ok {
		return ErrNotFound
	}
	order.CheckoutURL, order.QRExpiresAt = checkoutURL, expiresAt
	r.orders[orderNo] = order
	return nil
}

func (r *MemoryRepository) GetOrder(_ context.Context, userID, orderNo string) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	order, ok := r.orders[orderNo]
	if !ok || (userID != "" && order.UserID != userID) {
		return Order{}, ErrNotFound
	}
	if order.Status == "pending" && order.QRExpiresAt <= time.Now().UnixMilli() {
		order.Status = "expired"
		r.orders[orderNo] = order
	}
	return order, nil
}

func (r *MemoryRepository) ListUserOrders(_ context.Context, userID string, before int64, limit int) ([]Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Order{}
	for _, order := range r.orders {
		if order.UserID == userID && (before == 0 || order.CreatedAt < before) {
			out = append(out, order)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) ActivatePaidOrder(_ context.Context, orderNo, channel, tradeNo string, amount int64) (Order, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	order, ok := r.orders[orderNo]
	if !ok {
		return Order{}, false, ErrNotFound
	}
	if order.AmountMinor != amount || amount < 0 {
		return Order{}, false, ErrInvalidPayment
	}
	if order.Status == "activated" {
		if order.PayChannel != channel || order.PayTradeNo != tradeNo {
			return Order{}, false, ErrInvalidPayment
		}
		return order, false, nil
	}
	if order.Status != "pending" && order.Status != "paid" && order.Status != "expired" {
		return Order{}, false, ErrInvalidPayment
	}
	plan, ok := r.vipPlans[order.PlanID]
	if !ok {
		return Order{}, false, ErrNotFound
	}
	now := time.Now()
	account := r.usersByID[order.UserID]
	account.VIPExpiresAt = extendedVIPExpiry(account.VIPExpiresAt, plan.DurationDays, plan.Lifetime, now)
	r.usersByID[order.UserID] = account
	order.Status, order.PayChannel, order.PayTradeNo = "activated", channel, tradeNo
	order.PaidAt, order.ActivatedAt = now.UnixMilli(), now.UnixMilli()
	r.orders[orderNo] = order
	return order, true, nil
}

func (r *MemoryRepository) StoreActivationCodes(_ context.Context, codes []ActivationCode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, code := range codes {
		if _, exists := r.activationCodes[code.CodeHash]; exists {
			return errors.New("activation code collision")
		}
	}
	for _, code := range codes {
		r.activationCodes[code.CodeHash] = code
	}
	return nil
}

func (r *MemoryRepository) RedeemActivationCode(_ context.Context, userID, codeHash string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	code, ok := r.activationCodes[codeHash]
	if !ok || code.UsedBy != "" {
		return 0, ErrNotFound
	}
	days, lifetime := code.DurationDays, false
	if code.PlanID != "" {
		plan, exists := r.vipPlans[code.PlanID]
		if !exists || !plan.Enabled {
			return 0, ErrNotFound
		}
		days, lifetime = plan.DurationDays, plan.Lifetime
	}
	now := time.Now()
	account, exists := r.usersByID[userID]
	if !exists {
		return 0, ErrNotFound
	}
	account.VIPExpiresAt = extendedVIPExpiry(account.VIPExpiresAt, days, lifetime, now)
	r.usersByID[userID] = account
	code.UsedBy, code.UsedAt = userID, now.UnixMilli()
	r.activationCodes[codeHash] = code
	return account.VIPExpiresAt, nil
}

func (r *MemoryRepository) DailyCheckIn(_ context.Context, userID, date string, points int) (CheckInStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.checkIns[userID] == nil {
		r.checkIns[userID] = map[string]CheckIn{}
	}
	if _, exists := r.checkIns[userID][date]; exists {
		return CheckInStatus{}, ErrAlreadyCheckedIn
	}
	r.checkIns[userID][date] = CheckIn{Date: date, Points: points}
	account := r.pointsAccounts[userID]
	account.balance += int64(points)
	account.earned += int64(points)
	r.pointsAccounts[userID] = account
	r.appendMemoryPointsTransaction(userID, int64(points), account.balance, "check_in", date)
	return r.checkInStatusLocked(userID, date, points, 0), nil
}

func (r *MemoryRepository) GetCheckInStatus(_ context.Context, userID, date string, pointsPerCheckIn, pointsPerVIPDay int) (CheckInStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.checkInStatusLocked(userID, date, pointsPerCheckIn, pointsPerVIPDay), nil
}

func (r *MemoryRepository) checkInStatusLocked(userID, date string, checkPoints, exchange int) CheckInStatus {
	account := r.pointsAccounts[userID]
	items := make([]CheckIn, 0, len(r.checkIns[userID]))
	for _, item := range r.checkIns[userID] {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Date > items[j].Date })
	consecutive := consecutiveCheckInDays(items, date)
	if len(items) > 30 {
		items = items[:30]
	}
	_, today := r.checkIns[userID][date]
	return CheckInStatus{AvailablePoints: account.balance, TotalPoints: account.earned, UsedPoints: account.spent, CheckedInToday: today, PointsPerCheckIn: checkPoints, PointsPerVIPDay: exchange, TotalCheckIns: len(r.checkIns[userID]), ConsecutiveDays: consecutive, RecentCheckIns: items}
}

func (r *MemoryRepository) RedeemPointsForVIP(_ context.Context, userID string, days, pointsPerDay int) (int64, error) {
	if pointsPerDay <= 0 {
		return 0, ErrRedemptionDisabled
	}
	if days < 1 || days > 3650 {
		return 0, errors.New("invalid redemption days")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cost := int64(days * pointsPerDay)
	points := r.pointsAccounts[userID]
	if points.balance < cost {
		return 0, ErrInsufficientPoints
	}
	account, ok := r.usersByID[userID]
	if !ok {
		return 0, ErrNotFound
	}
	points.balance -= cost
	points.spent += cost
	r.pointsAccounts[userID] = points
	account.VIPExpiresAt = extendedVIPExpiry(account.VIPExpiresAt, days, false, time.Now())
	r.usersByID[userID] = account
	r.appendMemoryPointsTransaction(userID, -cost, points.balance, "redeem_vip", "")
	return account.VIPExpiresAt, nil
}

func (r *MemoryRepository) ListPointsTransactions(_ context.Context, userID string, before int64, limit int) ([]PointsTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.pointsTransactions[userID]
	out := []PointsTransaction{}
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		if before == 0 || items[i].ID < before {
			out = append(out, items[i])
		}
	}
	return out, nil
}

func (r *MemoryRepository) PointsLeaderboard(_ context.Context, date string, limit int) ([]PointsLeaderboardEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []PointsLeaderboardEntry{}
	for userID, account := range r.pointsAccounts {
		user := r.usersByID[userID]
		entry := PointsLeaderboardEntry{UserID: userID, DisplayName: user.DisplayName, Points: account.balance, CheckIns: len(r.checkIns[userID])}
		if date != "" {
			if _, ok := r.checkIns[userID][date]; !ok {
				continue
			}
			entry.CheckIns = 1
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Points > out[j].Points })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) appendMemoryPointsTransaction(userID string, change, balance int64, kind, ref string) {
	tx := PointsTransaction{ID: r.nextPointsTransaction, Change: change, BalanceAfter: balance, Type: kind, RefID: ref, CreatedAt: time.Now().UnixMilli()}
	r.nextPointsTransaction++
	r.pointsTransactions[userID] = append(r.pointsTransactions[userID], tx)
}

func extendedVIPExpiry(current int64, days int, lifetime bool, now time.Time) int64 {
	if lifetime {
		return time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC).UnixMilli()
	}
	base := now
	if current > now.UnixMilli() {
		base = time.UnixMilli(current)
	}
	return base.AddDate(0, 0, days).UnixMilli()
}

func consecutiveCheckInDays(items []CheckIn, today string) int {
	day, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	set := map[string]bool{}
	for _, item := range items {
		set[item.Date] = true
	}
	count := 0
	for set[day.Format("2006-01-02")] {
		count++
		day = day.AddDate(0, 0, -1)
	}
	return count
}
