package app

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

const monthlyMembershipPoints = 30
const monthlyMembershipDays = 30

func isLifetimeMembership(expiry int64) bool {
	return expiry >= time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
}

var redemptionRequestPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`)

type MembershipReward struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Points int    `json:"points"`
	Days   int    `json:"days"`
}

type MembershipRedemption struct {
	RequestID    string `json:"request_id"`
	PointsCost   int    `json:"points_cost"`
	DurationDays int    `json:"duration_days"`
	BalanceAfter int64  `json:"balance_after"`
	VIPExpiresAt int64  `json:"vip_expires_at"`
	CreatedAt    int64  `json:"created_at"`
	Replayed     bool   `json:"replayed"`
}

func withMembershipReward(status CheckInStatus, now time.Time) CheckInStatus {
	status.MonthlyReward = &MembershipReward{ID: "monthly", Title: "PLUS 会员月卡", Points: monthlyMembershipPoints, Days: monthlyMembershipDays}
	status.CheckInDate = utcDate(now)
	date := now.UTC()
	status.NextCheckInAt = time.Date(date.Year(), date.Month(), date.Day()+1, 0, 0, 0, 0, time.UTC).UnixMilli()
	return status
}

func (s *Server) redeemMonthlyMembership(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		RequestID string `json:"request_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !redemptionRequestPattern.MatchString(request.RequestID) {
		writeError(w, http.StatusBadRequest, errors.New("兑换请求标识无效，请重新发起兑换"))
		return
	}
	receipt, err := s.repo.RedeemMonthlyMembership(r.Context(), user.ID, request.RequestID)
	if errors.Is(err, ErrInsufficientPoints) {
		writeError(w, http.StatusBadRequest, errors.New("积分不足，兑换月卡需要 30 积分"))
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: receipt, Msg: "redeemed"})
}

func (r *MemoryRepository) RedeemMonthlyMembership(_ context.Context, userID, requestID string) (MembershipRedemption, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := userID + ":" + requestID
	if receipt, ok := r.membershipRedemptions[key]; ok {
		receipt.Replayed = true
		return receipt, nil
	}
	user, ok := r.usersByID[userID]
	if !ok {
		return MembershipRedemption{}, ErrNotFound
	}
	if isLifetimeMembership(user.VIPExpiresAt) {
		return MembershipRedemption{}, errors.New("当前为长期会员，无需兑换月卡")
	}
	points := r.pointsAccounts[userID]
	if points.balance < monthlyMembershipPoints {
		return MembershipRedemption{}, ErrInsufficientPoints
	}
	now := time.Now()
	receipt := MembershipRedemption{RequestID: requestID, PointsCost: monthlyMembershipPoints, DurationDays: monthlyMembershipDays, BalanceAfter: points.balance - monthlyMembershipPoints, VIPExpiresAt: extendedVIPExpiry(user.VIPExpiresAt, monthlyMembershipDays, false, now), CreatedAt: now.UnixMilli()}
	points.balance = receipt.BalanceAfter
	points.spent += monthlyMembershipPoints
	user.VIPExpiresAt = receipt.VIPExpiresAt
	r.pointsAccounts[userID] = points
	r.usersByID[userID] = user
	if r.membershipRedemptions == nil {
		r.membershipRedemptions = make(map[string]MembershipRedemption)
	}
	r.membershipRedemptions[key] = receipt
	r.appendMemoryPointsTransaction(userID, -monthlyMembershipPoints, points.balance, "redeem_vip", "monthly:"+requestID)
	return receipt, nil
}

func (r *PostgresRepository) RedeemMonthlyMembership(ctx context.Context, userID, requestID string) (MembershipRedemption, error) {
	// The user lock serializes receipts for the same account. At READ COMMITTED,
	// a waiter sees a committed receipt before it can debit again.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return MembershipRedemption{}, err
	}
	defer tx.Rollback(ctx)
	var current int64
	err = tx.QueryRow(ctx, `SELECT COALESCE((extract(epoch FROM vip_expires_at)*1000)::bigint,0) FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return MembershipRedemption{}, ErrNotFound
	}
	if err != nil {
		return MembershipRedemption{}, err
	}
	var receipt MembershipRedemption
	err = tx.QueryRow(ctx, `SELECT request_id,points_cost,duration_days,balance_after,vip_expires_at,created_at FROM membership_redemptions WHERE user_id=$1 AND request_id=$2`, userID, requestID).Scan(&receipt.RequestID, &receipt.PointsCost, &receipt.DurationDays, &receipt.BalanceAfter, &receipt.VIPExpiresAt, &receipt.CreatedAt)
	if err == nil {
		receipt.Replayed = true
		return receipt, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MembershipRedemption{}, err
	}
	if isLifetimeMembership(current) {
		return MembershipRedemption{}, errors.New("当前为长期会员，无需兑换月卡")
	}
	var balance int64
	err = tx.QueryRow(ctx, `UPDATE points_accounts SET balance=balance-$2,total_spent=total_spent+$2,updated_at=now() WHERE user_id=$1 AND balance>=$2 RETURNING balance`, userID, monthlyMembershipPoints).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return MembershipRedemption{}, ErrInsufficientPoints
	}
	if err != nil {
		return MembershipRedemption{}, err
	}
	now := time.Now()
	receipt = MembershipRedemption{RequestID: requestID, PointsCost: monthlyMembershipPoints, DurationDays: monthlyMembershipDays, BalanceAfter: balance, VIPExpiresAt: extendedVIPExpiry(current, monthlyMembershipDays, false, now), CreatedAt: now.UnixMilli()}
	if _, err = tx.Exec(ctx, `UPDATE users SET vip_expires_at=to_timestamp($2/1000.0) WHERE id=$1`, userID, receipt.VIPExpiresAt); err != nil {
		return MembershipRedemption{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO membership_redemptions(user_id,request_id,points_cost,duration_days,balance_after,vip_expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, userID, requestID, receipt.PointsCost, receipt.DurationDays, balance, receipt.VIPExpiresAt, receipt.CreatedAt); err != nil {
		return MembershipRedemption{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO points_transactions(user_id,change,balance_after,type,ref_id) VALUES($1,$2,$3,'redeem_vip',$4)`, userID, -monthlyMembershipPoints, balance, "monthly:"+requestID); err != nil {
		return MembershipRedemption{}, err
	}
	return receipt, tx.Commit(ctx)
}
