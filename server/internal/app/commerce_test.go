package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestCommercePaymentCodesCheckInAndRoomEntitlement(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	tokens, _ := NewTokenManager("commerce-test-secret-with-at-least-32-characters", "commerce-test")
	auth := NewAuthService(repo, tokens)
	adminDevice := DeviceInfo{ID: "commerce-admin-device", Label: "Admin", Platform: "test"}
	userDevice := DeviceInfo{ID: "commerce-user-device", Label: "User", Platform: "test"}
	otherDevice := DeviceInfo{ID: "commerce-other-device", Label: "Other", Platform: "test"}
	admin, err := auth.Register(ctx, "admin@commerce.test", "Admin", "correct horse battery", adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetUserAdmin(ctx, admin.User.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := repo.VerifyEmail(ctx, admin.User.ID); err != nil {
		t.Fatal(err)
	}
	admin, err = auth.Login(ctx, "admin@commerce.test", "correct horse battery", adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	user, _ := auth.Register(ctx, "user@commerce.test", "User", "another strong password", userDevice)
	other, _ := auth.Register(ctx, "other@commerce.test", "Other", "third strong password", otherDevice)
	provider, _ := NewHMACPaymentProvider("testpay", "https://pay.test/checkout", "commerce-callback-secret-with-32-characters")
	fixedNow := time.Unix(2000000000, 0)
	provider.now = func() time.Time { return fixedNow }
	server := NewServer(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}, Repository: repo, Auth: auth, Payment: provider, PointsPerCheckIn: 2, PointsPerVIPDay: 2})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	vipBody := requestJSON(t, http.MethodGet, httpServer.URL+"/api/v1/vip", "", nil)
	var vip struct {
		Data VIPInfo `json:"data"`
	}
	_ = json.Unmarshal(vipBody, &vip)
	if len(vip.Data.Plans) != 3 || vip.Data.PaymentMethod != "testpay" {
		t.Fatalf("vip info=%+v", vip.Data)
	}
	orderBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/orders", user.AccessToken, map[string]string{"plan_id": "monthly"}, userDevice)
	var created struct {
		Data Order `json:"data"`
	}
	_ = json.Unmarshal(orderBody, &created)
	if created.Data.AmountMinor != 800 || created.Data.CheckoutURL == "" {
		t.Fatalf("order=%+v", created.Data)
	}
	if got := requestStatusDevice(t, http.MethodGet, httpServer.URL+"/api/v1/orders/"+created.Data.OrderNo, other.AccessToken, nil, otherDevice); got != http.StatusNotFound {
		t.Fatalf("cross-user query status=%d", got)
	}
	if got := requestStatusDevice(t, http.MethodPut, httpServer.URL+"/api/v1/admin/users/"+other.User.ID+"/vip", user.AccessToken, map[string]int{"days": 5}, userDevice); got != http.StatusForbidden {
		t.Fatalf("non-admin VIP adjustment status=%d", got)
	}
	requestJSONDevice(t, http.MethodPut, httpServer.URL+"/api/v1/admin/users/"+other.User.ID+"/vip", admin.AccessToken, map[string]int{"days": 5}, adminDevice)
	adjusted, _ := repo.UserByID(ctx, other.User.ID)
	if adjusted.VIPExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("admin VIP adjustment was not applied")
	}
	callback := map[string]any{"order_no": created.Data.OrderNo, "trade_no": "trade-1", "amount_minor": int64(800), "timestamp": fixedNow.Unix()}
	callback["signature"] = "bad"
	if got := requestStatusDevice(t, http.MethodPost, httpServer.URL+"/api/v1/payments/callback", "", callback, DeviceInfo{}); got != http.StatusUnauthorized {
		t.Fatalf("bad callback status=%d", got)
	}
	callback["signature"] = provider.sign(created.Data.OrderNo, "trade-1", 800, fixedNow.Unix())
	paidBody := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/payments/callback", "", callback)
	var paid struct {
		Data struct {
			Activated bool `json:"activated"`
		} `json:"data"`
	}
	_ = json.Unmarshal(paidBody, &paid)
	if !paid.Data.Activated {
		t.Fatal("first callback did not activate")
	}
	repeatBody := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/payments/callback", "", callback)
	_ = json.Unmarshal(repeatBody, &paid)
	if paid.Data.Activated {
		t.Fatal("duplicate callback activated twice")
	}
	account, _ := repo.UserByID(ctx, user.User.ID)
	firstExpiry := account.VIPExpiresAt
	if firstExpiry <= time.Now().UnixMilli() {
		t.Fatalf("vip expiry=%d", firstExpiry)
	}

	requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/check-ins", user.AccessToken, nil, userDevice)
	if got := requestStatusDevice(t, http.MethodPost, httpServer.URL+"/api/v1/check-ins", user.AccessToken, nil, userDevice); got != http.StatusBadRequest {
		t.Fatalf("duplicate check-in status=%d", got)
	}
	requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/points/redeem-vip", user.AccessToken, map[string]int{"days": 1}, userDevice)
	account, _ = repo.UserByID(ctx, user.User.ID)
	if account.VIPExpiresAt <= firstExpiry {
		t.Fatalf("points did not extend VIP: before=%d after=%d", firstExpiry, account.VIPExpiresAt)
	}

	codesBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/admin/activation-codes", admin.AccessToken, map[string]any{"duration_days": 7, "count": 1}, adminDevice)
	var codes struct {
		Data struct {
			Codes []IssuedActivationCode `json:"codes"`
		} `json:"data"`
	}
	_ = json.Unmarshal(codesBody, &codes)
	if len(codes.Data.Codes) != 1 {
		t.Fatalf("codes=%s", codesBody)
	}
	plain := codes.Data.Codes[0].Code
	requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/activation-codes/redeem", other.AccessToken, map[string]string{"code": plain}, otherDevice)
	if got := requestStatusDevice(t, http.MethodPost, httpServer.URL+"/api/v1/activation-codes/redeem", user.AccessToken, map[string]string{"code": plain}, userDevice); got != http.StatusNotFound {
		t.Fatalf("reused code status=%d", got)
	}

	roomBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/rooms", user.AccessToken, map[string]any{"name": "VIP room", "source_url": "https://example.com/movie.mp4", "max_members": 4}, userDevice)
	var room struct {
		Data Room `json:"data"`
	}
	_ = json.Unmarshal(roomBody, &room)
	account, _ = repo.UserByID(ctx, user.User.ID)
	if room.Data.ExpiresAt != account.VIPExpiresAt {
		t.Fatalf("room expiry=%d vip=%d", room.Data.ExpiresAt, account.VIPExpiresAt)
	}
}

func TestPaymentSignatureWindowAndPayload(t *testing.T) {
	provider, err := NewHMACPaymentProvider("pay", "https://pay.test/checkout", "payment-signature-secret-with-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2000000000, 0)
	provider.now = func() time.Time { return now }
	signature := provider.sign("order", "trade", 800, now.Unix())
	if !provider.VerifyCallback("order", "trade", 800, now.Unix(), signature) {
		t.Fatal("valid signature rejected")
	}
	if provider.VerifyCallback("order", "trade", 801, now.Unix(), signature) || provider.VerifyCallback("order", "trade", 800, now.Add(-6*time.Minute).Unix(), provider.sign("order", "trade", 800, now.Add(-6*time.Minute).Unix())) {
		t.Fatal("tampered or stale callback accepted")
	}
}

func TestPostgresCommerceTransactions(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repo, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatal(err)
	}
	tokens, _ := NewTokenManager("postgres-commerce-secret-with-32-characters", "postgres-commerce")
	auth := NewAuthService(repo, tokens)
	device := DeviceInfo{ID: "postgres-commerce-device", Label: "Test", Platform: "test"}
	user, err := auth.Register(ctx, "pg-commerce@example.test", "Commerce", "correct horse battery", device)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repo.GetVIPPlan(ctx, "monthly")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	order, err := repo.CreateOrder(ctx, Order{OrderNo: "PG-COMMERCE-1", UserID: user.User.ID, PlanID: plan.ID, PlanTitle: plan.Title, AmountMinor: plan.PriceMinor, Currency: "CNY", Status: "pending", PayChannel: "test", CheckoutURL: "https://pay.test", QRExpiresAt: now.Add(3 * time.Minute).UnixMilli()})
	if err != nil || order.OrderNo == "" {
		t.Fatalf("order=%+v error=%v", order, err)
	}
	paid, activated, err := repo.ActivatePaidOrder(ctx, order.OrderNo, "test", "trade-pg-1", plan.PriceMinor)
	if err != nil || !activated || paid.Status != "activated" {
		t.Fatalf("paid=%+v activated=%v error=%v", paid, activated, err)
	}
	_, activated, err = repo.ActivatePaidOrder(ctx, order.OrderNo, "test", "trade-pg-1", plan.PriceMinor)
	if err != nil || activated {
		t.Fatalf("idempotent activated=%v error=%v", activated, err)
	}
	hash, _ := activationCodeHash("POSTGRESCODE123456")
	if err := repo.StoreActivationCodes(ctx, []ActivationCode{{CodeHash: hash, BatchID: "pg-batch", DurationDays: 7}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RedeemActivationCode(ctx, user.User.ID, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RedeemActivationCode(ctx, user.User.ID, hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reused code error=%v", err)
	}
	date := utcDate(now)
	if _, err := repo.DailyCheckIn(ctx, user.User.ID, date, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DailyCheckIn(ctx, user.User.ID, date, 3); !errors.Is(err, ErrAlreadyCheckedIn) {
		t.Fatalf("duplicate check-in error=%v", err)
	}
	if _, err := repo.RedeemPointsForVIP(ctx, user.User.ID, 1, 3); err != nil {
		t.Fatal(err)
	}
	transactions, err := repo.ListPointsTransactions(ctx, user.User.ID, 0, 10)
	if err != nil || len(transactions) != 2 || transactions[0].BalanceAfter != 0 {
		t.Fatalf("transactions=%+v error=%v", transactions, err)
	}
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("TEST_DATABASE_URL")
	if value == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	return value
}
