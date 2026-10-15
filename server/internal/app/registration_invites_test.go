package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type failRegistrationMailer struct{}

func (failRegistrationMailer) Send(context.Context, MailMessage) error {
	return errors.New("registration must not send mail while email verification is paused")
}

func TestRegistrationInvites(t *testing.T) { testRegistrationInvites(t, NewMemoryRepository()) }
func TestPostgresRegistrationInvites(t *testing.T) {
	ctx := context.Background()
	repo, err := OpenPostgres(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.pool.Exec(ctx, "TRUNCATE users,admin_documents,registration_invites,registration_quotas CASCADE"); err != nil {
		t.Fatal(err)
	}
	defer repo.pool.Exec(ctx, "TRUNCATE users,admin_documents,registration_invites,registration_quotas CASCADE")
	testRegistrationInvites(t, repo)
}

func testRegistrationInvites(t *testing.T, repo Repository) {
	ctx := context.Background()
	f := newManagementFixture(t, repo)
	f.server.options.RequireAccount = true
	f.auth.mailer = failRegistrationMailer{}
	f.call(t, "GET", "/api/v1/admin/registration-invites", nil, "", 401)
	f.call(t, "POST", "/api/v1/admin/registration-invites", map[string]any{"count": 1}, f.member.AccessToken, 403)
	f.call(t, "POST", "/api/v1/admin/registration-invites", map[string]any{"count": 101}, f.root.AccessToken, 400)
	f.call(t, "POST", "/api/v1/admin/registration-invites", map[string]any{"count": 1, "expires_days": 366}, f.root.AccessToken, 400)
	var issued struct {
		BatchID string                      `json:"batch_id"`
		Codes   []struct{ ID, Code string } `json:"codes"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/admin/registration-invites", map[string]any{"count": 5, "expires_days": 30, "label": "体验注册"}, f.root.AccessToken, 201), &issued)
	if len(issued.Codes) != 5 || issued.BatchID == "" {
		t.Fatal("batch not generated")
	}
	seen := map[string]bool{}
	for _, item := range issued.Codes {
		if len(registrationInviteHash(item.Code)) != 64 || seen[item.Code] {
			t.Fatal("invalid or repeated invite")
		}
		seen[item.Code] = true
	}
	listing := f.call(t, "GET", "/api/v1/admin/registration-invites", nil, f.root.AccessToken, 200)
	for _, item := range issued.Codes {
		if strings.Contains(string(listing), item.Code) || strings.Contains(string(listing), registrationInviteHash(item.Code)) {
			t.Fatal("admin list leaked invite secret")
		}
	}
	setPolicy := func(required bool) {
		t.Helper()
		var current struct {
			Version int64      `json:"version"`
			Policy  SitePolicy `json:"policy"`
		}
		json.Unmarshal(f.call(t, "GET", "/api/v1/admin/site-policy", nil, f.root.AccessToken, 200), &current)
		current.Policy.RegistrationRequiresInvite = required
		// This scenario exercises invitation reuse and approval with several
		// accounts from one test client; quota boundaries have their own tests.
		current.Policy.RegistrationDeviceLimit = 20
		f.call(t, "PUT", "/api/v1/admin/site-policy", map[string]any{"version": current.Version, "policy": current.Policy}, f.root.AccessToken, 200)
	}
	setPolicy(true)
	var options struct {
		Invite bool `json:"registration_invite_required"`
		Email  bool `json:"registration_email_code_required"`
	}
	json.Unmarshal(f.call(t, "GET", "/api/v1/auth/options", nil, "", 200), &options)
	if !options.Invite || options.Email {
		t.Fatal("public requirements disagree with registration policy")
	}
	register := func(email, code string, status int) json.RawMessage {
		t.Helper()
		return f.call(t, "POST", "/api/v1/auth/register", map[string]string{"email": email, "display_name": "Invited user", "password": "correct registration password", "invite_code": code}, "", status)
	}
	register("missing@invite.test", "", 400)
	register("invalid@invite.test", "not-a-code", 400)
	f.call(t, "POST", "/api/v1/auth/register/code", map[string]string{"email": "invited@invite.test"}, "", 403)
	var account Session
	json.Unmarshal(register("invited@invite.test", strings.ToLower(issued.Codes[0].Code), 201), &account)
	if account.AccessToken == "" || account.User.EmailVerified {
		t.Fatal("invite registration must not certify an email address")
	}
	f.call(t, "GET", "/api/v1/public/rooms", nil, "", 401)
	f.call(t, "GET", "/api/v1/public/rooms", nil, account.AccessToken, 200)
	register("replay@invite.test", issued.Codes[0].Code, 400)
	register(f.member.User.Email, issued.Codes[1].Code, 409)
	register("after-duplicate@invite.test", issued.Codes[1].Code, 201)
	f.call(t, "POST", "/api/v1/admin/registration-invites/"+issued.Codes[2].ID+"/revoke", nil, f.root.AccessToken, 200)
	register("revoked@invite.test", issued.Codes[2].Code, 400)
	f.call(t, "POST", "/api/v1/admin/registration-invites/"+issued.Codes[0].ID+"/revoke", nil, f.root.AccessToken, 409)
	expiredRaw, _ := randomActivationCode(20)
	expiredCode := "JOIN-" + expiredRaw
	if err := repo.StoreRegistrationInvites(ctx, f.root.User.ID, []RegistrationInvite{{ID: mustRandomString(12), CodeHash: registrationInviteHash(expiredCode), Hint: "expired", BatchID: "expired-batch", CreatedBy: f.root.User.ID, CreatedAt: time.Now().Add(-time.Hour).UnixMilli(), ExpiresAt: time.Now().Add(-time.Second).UnixMilli()}}); err != nil {
		t.Fatal(err)
	}
	register("expired@invite.test", expiredCode, 400)
	setPolicy(false)
	register("open@invite.test", "", 201)
	items, _, err := repo.FindRegistrationInvites(ctx, AdminQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == issued.Codes[3].ID && item.UsedAt != 0 {
			t.Fatal("open registration consumed an invite")
		}
	}
	setPolicy(true)
	// Account creation and one-time consumption share the same repository transaction.
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, e := repo.RegisterAccount(ctx, AccountRecord{User: User{ID: mustRandomString(12), Email: fmt.Sprintf("race-%d@invite.test", index), DisplayName: "Concurrent", SessionVersion: 1}, PasswordHash: "fixture-hash", CreatedAt: time.Now()}, false, registrationInviteHash(issued.Codes[3].Code))
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrRegistrationInvite) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("invite created %d accounts", wins)
	}
	items, total, err := repo.FindRegistrationInvites(ctx, AdminQuery{Status: "used", Limit: 100})
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("used filter: %d %v", total, err)
	}
	// Registration review still consumes the invite, but does not return a login session.
	policy, document, _ := loadSitePolicy(ctx, repo)
	policy.RegistrationRequiresReview = true
	raw, _ := json.Marshal(policy)
	if _, err = repo.PutAdminDocument(ctx, AdminDocument{Key: "site_policy", Version: document.Version, Data: raw}, f.root.User.ID); err != nil {
		t.Fatal(err)
	}
	register("pending@invite.test", issued.Codes[4].Code, 202)
	register("pending-reuse@invite.test", issued.Codes[4].Code, 400)
}
