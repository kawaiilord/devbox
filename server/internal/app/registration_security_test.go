package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegistrationSourceAndImmutableEnvironment(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	f.server.options.PublicBaseURL = "https://sameframe.test"
	client := base64.StdEncoding.EncodeToString([]byte(`{"timezone":"Asia/Shanghai","timezone_offset":480,"language":"zh-CN","platform":"Windows","screen_width":1440,"screen_height":900,"pixel_ratio":1}`))
	request := func(email, peer, forwarded, device string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "display_name": "Registration test", "password": "correct registration password"})
		r := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
		r.RemoteAddr = peer
		r.Header.Set("X-Device-ID", device)
		r.Header.Set("X-Device-Platform", "web")
		r.Header.Set("X-Real-IP", forwarded)
		r.Header.Set("X-Forwarded-For", "1.1.1.1")
		r.Header.Set("CF-Connecting-IP", "1.1.1.1")
		r.Header.Set("User-Agent", "Registration test browser")
		r.Header.Set("X-Client-Context", client)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		f.server.Handler().ServeHTTP(w, r)
		return w
	}
	w := request("observed@registration.test", "198.51.100.21:1200", "203.0.113.8", "registration-device-00001", nil)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct{ Data Session }
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Data.User.ID
	detail, err := f.repo.ReadRegistration(context.Background(), id)
	if err != nil || detail.Record.SourceIP != "198.51.100.21" || detail.Record.IPSource != "direct" || detail.Record.Client.Timezone != "Asia/Shanghai" || detail.Record.ContextStatus != "client_reported" {
		t.Fatalf("wrong trusted source/environment: %+v %v", detail.Record, err)
	}
	if strings.Contains(w.Body.String(), "source_ip") || strings.Contains(w.Body.String(), "device_hash") {
		t.Fatal("private registration metadata leaked into a public session")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("registration browser cookie is not protected")
	}
	original := *detail.Record
	_, err = f.auth.Login(context.Background(), "observed@registration.test", "correct registration password", DeviceInfo{ID: "changed-login-device-00001", Label: "Changed", Platform: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ = f.repo.ReadRegistration(context.Background(), id)
	if *detail.Record != original {
		t.Fatal("login replaced immutable registration evidence")
	}
	path := "/api/v1/admin/managed-users/" + id + "/registration"
	f.call(t, "GET", path, nil, "", 401)
	f.call(t, "GET", path, nil, f.member.AccessToken, 403)
	f.call(t, "GET", path, nil, f.operator.AccessToken, 403)
	f.call(t, "GET", path, nil, f.root.AccessToken, 200)
	legacy, _ := f.repo.ReadRegistration(context.Background(), f.member.User.ID)
	if legacy.Record != nil {
		t.Fatal("invented registration metadata for an older account")
	}
	// Keeping the signed browser cookie still joins accounts when X-Device-ID
	// and network address are changed between registrations.
	for i := 2; i <= 4; i++ {
		res := request(fmt.Sprintf("browser-%d@registration.test", i), fmt.Sprintf("198.51.100.%d:1200", 20+i), "", fmt.Sprintf("rotated-device-0000%d", i), cookies[0])
		want := 201
		if i == 4 {
			want = 429
		}
		if res.Code != want {
			t.Fatal(i, res.Code, res.Body.String())
		}
		if i == 4 && res.Header().Get("Retry-After") == "" {
			t.Fatal("missing quota retry interval")
		}
	}
	detail, _ = f.repo.ReadRegistration(context.Background(), id)
	if detail.RelatedTotal != 2 || len(detail.Related) != 2 {
		t.Fatalf("browser correlation missing: %+v", detail)
	}
}

func TestRegistrationProxyAndIPv6(t *testing.T) {
	s := NewServer(Options{TrustedProxyCIDRs: []string{"172.19.0.0/16"}})
	for _, tc := range []struct{ peer, header, want, source string }{
		{"172.19.0.6:5000", "203.0.113.4", "203.0.113.4", "trusted_proxy"},
		{"[::ffff:198.51.100.2]:5000", "203.0.113.4", "198.51.100.2", "direct"},
		{"172.19.0.6:5000", "bad-header", "172.19.0.6", "unverified_proxy"},
		{"[2001:db8:1:2::ab]:5000", "", "2001:db8:1:2::ab", "direct"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Real-IP", tc.header)
		h := s.withTrustedProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record := s.registrationFromRequest(w, r, DeviceInfo{ID: "valid-test-device-00001"}, "password")
			if record.SourceIP != tc.want || record.IPSource != tc.source {
				t.Fatalf("wrong proxy provenance: %+v", record)
			}
			if (validateRegistrationRecord(record) != nil) != (tc.source == "unverified_proxy") {
				t.Fatal("missing proxy address did not fail closed")
			}
		}))
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	if registrationNetwork("2001:db8:1:2::1") != registrationNetwork("2001:db8:1:2:abcd::99") || registrationNetwork("::ffff:203.0.113.5") != "203.0.113.5" {
		t.Fatal("address rotation bypasses a network quota")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Client-Context", strings.Repeat("A", 3000))
	r.AddCookie(&http.Cookie{Name: registrationBrowserCookie, Value: "attacker-controlled-cookie-without-valid-signature"})
	w := httptest.NewRecorder()
	record := s.registrationFromRequest(w, r, DeviceInfo{ID: "valid-test-device-00001"}, "password")
	if record.ContextStatus != "invalid" || len(w.Result().Cookies()) != 1 {
		t.Fatal("invalid context or forged browser token was accepted")
	}
}

func TestRegistrationQuotasMemory(t *testing.T) {
	testRegistrationQuotas(t, NewMemoryRepository())
}

func TestRegistrationQuotasPostgres(t *testing.T) {
	ctx := context.Background()
	repo, err := OpenPostgres(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.pool.Exec(ctx, "TRUNCATE users,admin_documents,registration_quotas CASCADE"); err != nil {
		t.Fatal(err)
	}
	defer repo.pool.Exec(ctx, "TRUNCATE users,admin_documents,registration_quotas CASCADE")
	testRegistrationQuotas(t, repo)
}

func testRegistrationQuotas(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	makeAccount := func(ip, device, method string) AccountRecord {
		id := mustRandomString(16)
		return AccountRecord{User: User{ID: id, Email: strings.ToLower(id) + "@registration.test", DisplayName: "Quota fixture", SessionVersion: 1}, PasswordHash: "fixture-hash", CreatedAt: time.Now(),
			Registration: &RegistrationRecord{Method: method, SourceIP: ip, PeerIP: ip, IPSource: "direct", Network: registrationNetwork(ip), DeviceHash: hashDeviceID(device)}}
	}
	var wg sync.WaitGroup
	results := make(chan error, 18)
	for i := 0; i < 18; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := makeAccount("198.51.100.70", fmt.Sprintf("race-device-%d", i), "password")
			_, err := repo.RegisterAccount(ctx, a, false)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	allowed := 0
	for err := range results {
		var limit *registrationLimitError
		if err == nil {
			allowed++
		} else if !errors.As(err, &limit) || limit.Bucket != "network" {
			t.Fatal("unexpected concurrent registration error", err)
		}
	}
	if allowed != 5 {
		t.Fatal("concurrent requests exceeded the source quota", allowed)
	}
	// Password and OAuth registrations share the same durable device quota.
	for i := 1; i <= 4; i++ {
		a := makeAccount(fmt.Sprintf("198.51.100.%d", 80+i), "shared-oauth-device", "oauth:test")
		_, err := repo.RegisterAccount(ctx, a, i == 1)
		var limit *registrationLimitError
		if i <= 3 && err != nil || i == 4 && (!errors.As(err, &limit) || limit.Bucket != "device") {
			t.Fatal("OAuth/device quota mismatch", i, err)
		}
		if err != nil {
			if _, lookup := repo.UserByEmail(ctx, a.Email); lookup == nil {
				t.Fatal("quota rejection left an account behind")
			}
		}
	}
	// A failed duplicate must not consume a successful-registration slot.
	a := makeAccount("198.51.100.100", "duplicate-device", "password")
	if _, err := repo.RegisterAccount(ctx, a, false); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterAccount(ctx, a, false); !errors.Is(err, ErrEmailExists) {
		t.Fatal("duplicate account error lost", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := repo.RegisterAccount(ctx, makeAccount("198.51.100.100", "duplicate-device", "password"), false); err != nil {
			t.Fatal("duplicate consumed quota", err)
		}
	}
	if _, err := repo.RegisterAccount(ctx, makeAccount("198.51.100.100", "duplicate-device", "password"), false); err == nil {
		t.Fatal("device quota was not enforced")
	}
	detail, err := repo.ReadRegistration(ctx, a.ID)
	if err != nil || detail.Record == nil || detail.Record.RegisteredAt == 0 || detail.RelatedTotal != 2 {
		t.Fatal("origin was not committed with the account", detail, err)
	}
	// Changing the entire IPv6 host part still belongs to one quota network.
	for i := 1; i <= 6; i++ {
		_, err := repo.RegisterAccount(ctx, makeAccount(fmt.Sprintf("2001:db8:77:1::%x", i), fmt.Sprintf("ipv6-device-%d", i), "password"), false)
		if (err == nil) != (i <= 5) {
			t.Fatal("IPv6 network quota mismatch", i, err)
		}
	}
	// Device bans are checked in the same account transaction, before an
	// account can be left behind by the subsequent session/device binding.
	device := DeviceInfo{ID: "banned-registration-device", Label: "Blocked", Platform: "test"}
	if err := repo.BindDevice(ctx, a.ID, hashDeviceID(device.ID), device); err != nil {
		t.Fatal(err)
	}
	if err := repo.BanDevice(ctx, hashDeviceID(device.ID), "test ban", a.ID); err != nil {
		t.Fatal(err)
	}
	blocked := makeAccount("198.51.100.120", device.ID, "password")
	if _, err := repo.RegisterAccount(ctx, blocked, false); !errors.Is(err, ErrDeviceBlocked) {
		t.Fatal("blocked device created an account", err)
	}
	if _, err := repo.UserByEmail(ctx, blocked.Email); err == nil {
		t.Fatal("blocked registration was partly committed")
	}
}

func TestRegistrationQuotaWindowResetsAndPolicyValidation(t *testing.T) {
	now := time.Now().UnixMilli()
	q := quotaNext(registrationQuota{Start: now - registrationWindow.Milliseconds(), Count: 20}, now)
	if q.Count != 1 || q.Start != now {
		t.Fatal("expired quota did not reset", q)
	}
	p := defaultSitePolicy()
	if p.RegistrationIPLimit != 5 || p.RegistrationDeviceLimit != 3 {
		t.Fatal("unsafe default signup quotas")
	}
	p.RegistrationIPLimit = -1
	if validateSitePolicy(p) == nil {
		t.Fatal("invalid quota setting accepted")
	}
}
