package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const orderSelectColumns = `order_no,user_id,plan_id,plan_title,amount_minor,currency,status,pay_channel,COALESCE(pay_trade_no,''),checkout_url,(extract(epoch FROM qr_expires_at)*1000)::bigint,(extract(epoch FROM created_at)*1000)::bigint,COALESCE((extract(epoch FROM paid_at)*1000)::bigint,0),COALESCE((extract(epoch FROM activated_at)*1000)::bigint,0)`

func (r *PostgresRepository) ListVIPPlans(ctx context.Context, includeDisabled bool) ([]VIPPlan, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,title,price_minor,original_price_minor,duration_days,lifetime,popular,enabled FROM vip_plans WHERE $1 OR enabled ORDER BY price_minor,id`, includeDisabled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VIPPlan{}
	for rows.Next() {
		var plan VIPPlan
		if err := rows.Scan(&plan.ID, &plan.Title, &plan.PriceMinor, &plan.OriginalPriceMinor, &plan.DurationDays, &plan.Lifetime, &plan.Popular, &plan.Enabled); err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetVIPPlan(ctx context.Context, id string) (VIPPlan, error) {
	var plan VIPPlan
	err := r.pool.QueryRow(ctx, `SELECT id,title,price_minor,original_price_minor,duration_days,lifetime,popular,enabled FROM vip_plans WHERE id=$1 AND enabled`, id).Scan(&plan.ID, &plan.Title, &plan.PriceMinor, &plan.OriginalPriceMinor, &plan.DurationDays, &plan.Lifetime, &plan.Popular, &plan.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return VIPPlan{}, ErrNotFound
	}
	return plan, err
}

func (r *PostgresRepository) CreateOrder(ctx context.Context, order Order) (Order, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO payment_orders(order_no,user_id,plan_id,plan_title,amount_minor,currency,status,pay_channel,checkout_url,qr_expires_at) VALUES($1,$2,$3,$4,$5,$6,'pending',$7,$8,to_timestamp($9/1000.0)) RETURNING (extract(epoch FROM created_at)*1000)::bigint`, order.OrderNo, order.UserID, order.PlanID, order.PlanTitle, order.AmountMinor, order.Currency, order.PayChannel, order.CheckoutURL, order.QRExpiresAt).Scan(&order.CreatedAt)
	return order, err
}

func (r *PostgresRepository) SetOrderCheckout(ctx context.Context, orderNo, checkoutURL string, expiresAt int64) error {
	command, err := r.pool.Exec(ctx, `UPDATE payment_orders SET checkout_url=$2,qr_expires_at=to_timestamp($3/1000.0) WHERE order_no=$1 AND status='pending'`, orderNo, checkoutURL, expiresAt)
	if err == nil && command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return err
}

func scanOrder(row pgx.Row) (Order, error) {
	var order Order
	err := row.Scan(&order.OrderNo, &order.UserID, &order.PlanID, &order.PlanTitle, &order.AmountMinor, &order.Currency, &order.Status, &order.PayChannel, &order.PayTradeNo, &order.CheckoutURL, &order.QRExpiresAt, &order.CreatedAt, &order.PaidAt, &order.ActivatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	return order, err
}

func (r *PostgresRepository) GetOrder(ctx context.Context, userID, orderNo string) (Order, error) {
	if _, err := r.pool.Exec(ctx, `UPDATE payment_orders SET status='expired' WHERE order_no=$1 AND status='pending' AND qr_expires_at<=now()`, orderNo); err != nil {
		return Order{}, err
	}
	return scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderSelectColumns+` FROM payment_orders WHERE order_no=$1 AND ($2='' OR user_id=$2)`, orderNo, userID))
}

func (r *PostgresRepository) ListUserOrders(ctx context.Context, userID string, before int64, limit int) ([]Order, error) {
	if _, err := r.pool.Exec(ctx, `UPDATE payment_orders SET status='expired' WHERE user_id=$1 AND status='pending' AND qr_expires_at<=now()`, userID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+orderSelectColumns+` FROM payment_orders WHERE user_id=$1 AND ($2::bigint=0 OR created_at<to_timestamp($2/1000.0)) ORDER BY created_at DESC LIMIT $3`, userID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ActivatePaidOrder(ctx context.Context, orderNo, channel, tradeNo string, amount int64) (Order, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Order{}, false, err
	}
	defer tx.Rollback(ctx)
	order, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderSelectColumns+` FROM payment_orders WHERE order_no=$1 FOR UPDATE`, orderNo))
	if err != nil {
		return Order{}, false, err
	}
	if order.AmountMinor != amount || amount < 0 || tradeNo == "" || channel == "" {
		return Order{}, false, ErrInvalidPayment
	}
	if order.Status == "activated" {
		if order.PayChannel != channel || order.PayTradeNo != tradeNo {
			return Order{}, false, ErrInvalidPayment
		}
		return order, false, tx.Commit(ctx)
	}
	if order.Status != "pending" && order.Status != "paid" && order.Status != "expired" {
		return Order{}, false, ErrInvalidPayment
	}
	var days int
	var lifetime bool
	if err := tx.QueryRow(ctx, `SELECT duration_days,lifetime FROM vip_plans WHERE id=$1`, order.PlanID).Scan(&days, &lifetime); err != nil {
		return Order{}, false, err
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE((extract(epoch FROM vip_expires_at)*1000)::bigint,0) FROM users WHERE id=$1 FOR UPDATE`, order.UserID).Scan(&current); err != nil {
		return Order{}, false, err
	}
	expires := extendedVIPExpiry(current, days, lifetime, time.Now())
	if _, err := tx.Exec(ctx, `UPDATE users SET vip_expires_at=to_timestamp($2/1000.0) WHERE id=$1`, order.UserID, expires); err != nil {
		return Order{}, false, err
	}
	err = tx.QueryRow(ctx, `UPDATE payment_orders SET status='activated',pay_channel=$2,pay_trade_no=$3,paid_at=COALESCE(paid_at,now()),activated_at=now() WHERE order_no=$1 RETURNING `+orderSelectColumns, orderNo, channel, tradeNo).Scan(&order.OrderNo, &order.UserID, &order.PlanID, &order.PlanTitle, &order.AmountMinor, &order.Currency, &order.Status, &order.PayChannel, &order.PayTradeNo, &order.CheckoutURL, &order.QRExpiresAt, &order.CreatedAt, &order.PaidAt, &order.ActivatedAt)
	if err != nil {
		return Order{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, false, err
	}
	return order, true, nil
}

func (r *PostgresRepository) StoreActivationCodes(ctx context.Context, codes []ActivationCode) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, code := range codes {
		if _, err := tx.Exec(ctx, `INSERT INTO activation_codes(code_hash,batch_id,plan_id,duration_days) VALUES($1,$2,NULLIF($3,''),$4)`, code.CodeHash, code.BatchID, code.PlanID, code.DurationDays); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) RedeemActivationCode(ctx context.Context, userID, codeHash string) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var planID string
	var days int
	var used bool
	err = tx.QueryRow(ctx, `SELECT COALESCE(plan_id,''),duration_days,used_by IS NOT NULL FROM activation_codes WHERE code_hash=$1 FOR UPDATE`, codeHash).Scan(&planID, &days, &used)
	if errors.Is(err, pgx.ErrNoRows) || used {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	lifetime := false
	if planID != "" {
		if err := tx.QueryRow(ctx, `SELECT duration_days,lifetime FROM vip_plans WHERE id=$1 AND enabled`, planID).Scan(&days, &lifetime); err != nil {
			return 0, err
		}
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE((extract(epoch FROM vip_expires_at)*1000)::bigint,0) FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return 0, err
	}
	expires := extendedVIPExpiry(current, days, lifetime, time.Now())
	if _, err := tx.Exec(ctx, `UPDATE users SET vip_expires_at=to_timestamp($2/1000.0) WHERE id=$1`, userID, expires); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE activation_codes SET used_by=$2,used_at=now() WHERE code_hash=$1`, codeHash, userID); err != nil {
		return 0, err
	}
	return expires, tx.Commit(ctx)
}

func (r *PostgresRepository) DailyCheckIn(ctx context.Context, userID, date string, points int) (CheckInStatus, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CheckInStatus{}, err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `INSERT INTO daily_check_ins(user_id,check_date,points) VALUES($1,$2::date,$3) ON CONFLICT DO NOTHING`, userID, date, points)
	if err != nil {
		return CheckInStatus{}, err
	}
	if command.RowsAffected() != 1 {
		return CheckInStatus{}, ErrAlreadyCheckedIn
	}
	var balance int64
	err = tx.QueryRow(ctx, `INSERT INTO points_accounts(user_id,balance,total_earned) VALUES($1,$2,$2) ON CONFLICT(user_id) DO UPDATE SET balance=points_accounts.balance+$2,total_earned=points_accounts.total_earned+$2,updated_at=now() RETURNING balance`, userID, points).Scan(&balance)
	if err != nil {
		return CheckInStatus{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO points_transactions(user_id,change,balance_after,type,ref_id) VALUES($1,$2,$3,'check_in',$4)`, userID, points, balance, date); err != nil {
		return CheckInStatus{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CheckInStatus{}, err
	}
	return r.GetCheckInStatus(ctx, userID, date, points, 0)
}

func (r *PostgresRepository) GetCheckInStatus(ctx context.Context, userID, date string, pointsPerCheckIn, pointsPerVIPDay int) (CheckInStatus, error) {
	status := CheckInStatus{PointsPerCheckIn: pointsPerCheckIn, PointsPerVIPDay: pointsPerVIPDay, RecentCheckIns: []CheckIn{}}
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(balance,0),COALESCE(total_earned,0),COALESCE(total_spent,0) FROM points_accounts WHERE user_id=$1`, userID).Scan(&status.AvailablePoints, &status.TotalPoints, &status.UsedPoints)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return status, err
	}
	rows, err := r.pool.Query(ctx, `SELECT check_date::text,points FROM daily_check_ins WHERE user_id=$1 ORDER BY check_date DESC LIMIT 3660`, userID)
	if err != nil {
		return status, err
	}
	all := []CheckIn{}
	for rows.Next() {
		var item CheckIn
		if err := rows.Scan(&item.Date, &item.Points); err != nil {
			rows.Close()
			return status, err
		}
		all = append(all, item)
	}
	rows.Close()
	status.TotalCheckIns = len(all)
	status.ConsecutiveDays = consecutiveCheckInDays(all, date)
	if len(all) > 30 {
		status.RecentCheckIns = all[:30]
	} else {
		status.RecentCheckIns = all
	}
	for _, item := range all {
		if item.Date == date {
			status.CheckedInToday = true
			break
		}
	}
	return status, nil
}

func (r *PostgresRepository) RedeemPointsForVIP(ctx context.Context, userID string, days, pointsPerDay int) (int64, error) {
	if pointsPerDay <= 0 {
		return 0, ErrRedemptionDisabled
	}
	if days < 1 || days > 3650 {
		return 0, errors.New("invalid redemption days")
	}
	cost := int64(days * pointsPerDay)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var balance int64
	err = tx.QueryRow(ctx, `UPDATE points_accounts SET balance=balance-$2,total_spent=total_spent+$2,updated_at=now() WHERE user_id=$1 AND balance>=$2 RETURNING balance`, userID, cost).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInsufficientPoints
	}
	if err != nil {
		return 0, err
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE((extract(epoch FROM vip_expires_at)*1000)::bigint,0) FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return 0, err
	}
	expires := extendedVIPExpiry(current, days, false, time.Now())
	if _, err := tx.Exec(ctx, `UPDATE users SET vip_expires_at=to_timestamp($2/1000.0) WHERE id=$1`, userID, expires); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO points_transactions(user_id,change,balance_after,type,ref_id) VALUES($1,$2,$3,'redeem_vip',$4)`, userID, -cost, balance, fmt.Sprint(days)); err != nil {
		return 0, err
	}
	return expires, tx.Commit(ctx)
}

func (r *PostgresRepository) ListPointsTransactions(ctx context.Context, userID string, before int64, limit int) ([]PointsTransaction, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,change,balance_after,type,ref_id,(extract(epoch FROM created_at)*1000)::bigint FROM points_transactions WHERE user_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, userID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PointsTransaction{}
	for rows.Next() {
		var item PointsTransaction
		if err := rows.Scan(&item.ID, &item.Change, &item.BalanceAfter, &item.Type, &item.RefID, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) PointsLeaderboard(ctx context.Context, date string, limit int) ([]PointsLeaderboardEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT u.id,u.display_name,p.balance,COUNT(c.check_date)::int FROM points_accounts p JOIN users u ON u.id=p.user_id LEFT JOIN daily_check_ins c ON c.user_id=p.user_id AND ($1='' OR c.check_date=$1::date) WHERE $1='' OR c.check_date IS NOT NULL GROUP BY u.id,u.display_name,p.balance ORDER BY p.balance DESC,u.id LIMIT $2`, date, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PointsLeaderboardEntry{}
	for rows.Next() {
		var item PointsLeaderboardEntry
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Points, &item.CheckIns); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
