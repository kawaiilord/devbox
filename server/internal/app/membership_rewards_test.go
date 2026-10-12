package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryMonthlyMembership(t *testing.T) { testMonthlyMembership(t, NewMemoryRepository()) }
func TestPostgresMonthlyMembership(t *testing.T) {
	repo, err := OpenPostgres(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	testMonthlyMembership(t, repo)
}

func testMonthlyMembership(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	create := func(name string) string {
		id := "rewards-" + name + "-" + mustRandomString(8)
		err := repo.CreateUser(ctx, AccountRecord{User: User{ID: id, Email: id + "@example.test", DisplayName: "Rewards Test"}, PasswordHash: "not-used-in-this-test", CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	user := create("monthly")
	if _, err := repo.RedeemMonthlyMembership(ctx, user, "request-empty-balance"); !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("insufficient points: %v", err)
	}
	account, _ := repo.UserByID(ctx, user)
	if account.VIPExpiresAt != 0 {
		t.Fatal("failed redemption granted membership")
	}
	if _, err := repo.DailyCheckIn(ctx, user, "2026-10-01", 60); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UnixMilli()
	first, err := repo.RedeemMonthlyMembership(ctx, user, "request-first-month")
	if err != nil {
		t.Fatal(err)
	}
	month := int64(30 * 24 * time.Hour / time.Millisecond)
	if first.PointsCost != 30 || first.DurationDays != 30 || first.BalanceAfter != 30 || first.VIPExpiresAt < before+month || first.VIPExpiresAt > time.Now().UnixMilli()+month {
		t.Fatalf("first receipt=%+v", first)
	}
	// Concurrent retries of one request must all return the same receipt.
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := repo.RedeemMonthlyMembership(ctx, user, "request-first-month")
			if err != nil || !receipt.Replayed || receipt.VIPExpiresAt != first.VIPExpiresAt {
				t.Errorf("replay=%+v err=%v", receipt, err)
			}
		}()
	}
	wg.Wait()
	second, err := repo.RedeemMonthlyMembership(ctx, user, "request-second-month")
	if err != nil || second.VIPExpiresAt != first.VIPExpiresAt+month || second.BalanceAfter != 0 {
		t.Fatalf("extension=%+v err=%v", second, err)
	}
	entries, err := repo.ListPointsTransactions(ctx, user, 0, 100)
	if err != nil || len(entries) != 3 || entries[0].Change != -30 || entries[0].BalanceAfter != 0 {
		t.Fatalf("ledger=%+v err=%v", entries, err)
	}
	// Multiple distinct requests competing for one remaining month cannot overdraft.
	limited := create("race")
	if _, err := repo.DailyCheckIn(ctx, limited, "2026-10-01", 30); err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.RedeemMonthlyMembership(ctx, limited, fmt.Sprintf("concurrent-request-%02d", i))
			outcomes <- err
		}(i)
	}
	wg.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrInsufficientPoints) {
			t.Errorf("concurrent error=%v", err)
		}
	}
	status, err := repo.GetCheckInStatus(ctx, limited, "2026-10-01", 1, 0)
	if err != nil || successes != 1 || status.AvailablePoints != 0 || status.UsedPoints != 30 {
		t.Fatalf("successes=%d status=%+v err=%v", successes, status, err)
	}
	// Each account owns its request namespace.
	other := create("other")
	if _, err := repo.DailyCheckIn(ctx, other, "2026-10-01", 30); err != nil {
		t.Fatal(err)
	}
	receipt, err := repo.RedeemMonthlyMembership(ctx, other, "request-first-month")
	if err != nil || receipt.Replayed {
		t.Fatalf("request namespace receipt=%+v err=%v", receipt, err)
	}
	lifetime := create("lifetime")
	if err := repo.SetUserVIP(ctx, lifetime, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DailyCheckIn(ctx, lifetime, "2026-10-01", 30); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RedeemMonthlyMembership(ctx, lifetime, "lifetime-no-charge"); err == nil {
		t.Fatal("lifetime member was charged")
	}
	lifetimeStatus, _ := repo.GetCheckInStatus(ctx, lifetime, "2026-10-01", 1, 0)
	if lifetimeStatus.AvailablePoints != 30 {
		t.Fatal("lifetime balance changed")
	}

	memberships, err := repo.ActiveMemberships(ctx, []string{user, limited, other, "missing-user"})
	if err != nil || !memberships[user] || !memberships[other] || memberships["missing-user"] {
		t.Fatalf("membership identities=%v err=%v", memberships, err)
	}
}

func TestMembershipRewardBoundary(t *testing.T) {
	now := time.Date(2026, 10, 12, 23, 59, 59, 0, time.UTC)
	status := withMembershipReward(CheckInStatus{}, now)
	if status.CheckInDate != "2026-10-12" || status.NextCheckInAt != time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC).UnixMilli() || status.MonthlyReward.Points != 30 || status.MonthlyReward.Days != 30 {
		t.Fatalf("status=%+v", status)
	}
	month := int64(30 * 24 * time.Hour / time.Millisecond)
	if extendedVIPExpiry(now.Add(-time.Hour).UnixMilli(), 30, false, now) != now.UnixMilli()+month {
		t.Fatal("expired membership must begin at redemption time")
	}
}

func TestCheckInStreakBeforeTodayAndAfterMissedDay(t *testing.T) {
	items := []CheckIn{{Date: "2026-10-10"}, {Date: "2026-10-11"}}
	if got := consecutiveCheckInDays(items, "2026-10-12"); got != 2 {
		t.Fatalf("unclaimed today streak=%d", got)
	}
	if got := consecutiveCheckInDays(items, "2026-10-13"); got != 0 {
		t.Fatalf("missed day streak=%d", got)
	}
	items = append(items, CheckIn{Date: "2026-10-12"})
	if got := consecutiveCheckInDays(items, "2026-10-12"); got != 3 {
		t.Fatalf("checked today streak=%d", got)
	}
}
