package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
)

func TestSecurityLifecycle(t *testing.T) { testSecurityLifecycle(t, NewMemoryRepository()) }
func TestPostgresSecurityLifecycle(t *testing.T) {
	repo, err := OpenPostgres(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.pool.Exec(context.Background(), "TRUNCATE users,admin_documents,auth_challenges CASCADE"); err != nil {
		t.Fatal(err)
	}
	defer repo.pool.Exec(context.Background(), "TRUNCATE users,admin_documents,auth_challenges CASCADE")
	testSecurityLifecycle(t, repo)
}
func testSecurityLifecycle(t *testing.T, repo Repository) {
	f := newManagementFixture(t, repo)
	ctx := context.Background()
	var setup struct {
		Challenge string `json:"challenge"`
		Secret    string `json:"secret"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/account/totp/setup", map[string]string{"password": "correct horse battery"}, f.member.AccessToken, 200), &setup)
	var enabled struct {
		Session Session  `json:"session"`
		Codes   []string `json:"recovery_codes"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/account/totp/confirm", map[string]string{"challenge": setup.Challenge, "code": totpAt(setup.Secret, time.Now().Unix()/30)}, f.member.AccessToken, 200), &enabled)
	if len(enabled.Codes) != 10 || enabled.Session.AccessToken == "" {
		t.Fatal("missing recovery codes or session")
	}
	f.call(t, "GET", "/api/v1/users/me", nil, f.member.AccessToken, 401)
	if _, e := f.auth.Refresh(ctx, f.member.RefreshToken, f.device); e == nil {
		t.Fatal("MFA activation left a refresh session alive")
	}
	state, e := repo.ReadAccountSecurity(ctx, f.member.User.ID)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(state.TOTPSecret, setup.Secret) || !strings.HasPrefix(state.TOTPSecret, "v1.") {
		t.Fatal("TOTP stored without encryption")
	}
	raw := f.call(t, "GET", "/api/v1/account/security", nil, enabled.Session.AccessToken, 200)
	if strings.Contains(string(raw), state.TOTPSecret) || strings.Contains(string(raw), state.RecoveryHashes[0]) {
		t.Fatal("security response leaked secret")
	}
	getChallenge := func() string {
		t.Helper()
		var c struct {
			Challenge string `json:"challenge"`
		}
		json.Unmarshal(f.call(t, "POST", "/api/v1/auth/login", map[string]string{"email": f.member.User.Email, "password": "correct horse battery"}, "", 202), &c)
		if c.Challenge == "" {
			t.Fatal("password-only login bypassed MFA")
		}
		return c.Challenge
	}
	challenge := getChallenge()
	f.call(t, "POST", "/api/v1/auth/mfa", map[string]string{"challenge": challenge, "code": enabled.Codes[0]}, "", 200)
	f.call(t, "POST", "/api/v1/auth/mfa", map[string]string{"challenge": challenge, "code": enabled.Codes[1]}, "", 401)
	f.call(t, "POST", "/api/v1/auth/mfa", map[string]string{"challenge": getChallenge(), "code": enabled.Codes[0]}, "", 401)
	state, _ = repo.ReadAccountSecurity(ctx, f.member.User.ID)
	if len(state.RecoveryHashes) != 9 {
		t.Fatal("recovery code wasn't consumed")
	}
	f.call(t, "POST", "/api/v1/account/totp/disable", map[string]string{"password": "correct horse battery", "code": enabled.Codes[1]}, enabled.Session.AccessToken, 200)
	if _, e = f.auth.Login(ctx, f.member.User.Email, "correct horse battery", f.device); e != nil {
		t.Fatal(e)
	}
	// Browser handoffs must be single-use and fenced by current session version.
	var start struct {
		URL   string `json:"url"`
		Token string `json:"poll_token"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/auth/browser/start", nil, "", 200), &start)
	u, _ := url.Parse(start.URL)
	bridge, _ := url.ParseQuery(u.Fragment)
	f.call(t, "POST", "/api/v1/auth/browser/exchange", map[string]string{"token": start.Token}, "", 200)
	f.call(t, "POST", "/api/v1/auth/browser/finish", map[string]string{"token": bridge.Get("bridge")}, f.root.AccessToken, 200)
	raw = f.call(t, "POST", "/api/v1/auth/browser/exchange", map[string]string{"token": start.Token}, "", 200)
	var session Session
	json.Unmarshal(raw, &session)
	if session.User.ID != f.root.User.ID {
		t.Fatal("handoff user mismatch")
	}
	f.call(t, "POST", "/api/v1/auth/browser/finish", map[string]string{"token": bridge.Get("bridge")}, f.root.AccessToken, 400)
	// Atomic challenge consumption under concurrent requests.
	token, e := saveChallenge(ctx, repo, "test", map[string]string{"value": "ok"})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := repo.ConsumeAuthChallenge(ctx, token, "test"); results <- e == nil }()
	}
	wg.Wait()
	close(results)
	count := 0
	for ok := range results {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatal("replayed challenge")
	}
}
func TestTOTPVector(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	if got := totpAt(secret, 1); got != "287082" {
		t.Fatal(got)
	}
}
func TestSMTPTransportAndSecretRedaction(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	received := make(chan string, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		fmt.Fprint(conn, "220 fixture ESMTP\r\n")
		scan := bufio.NewScanner(conn)
		data := false
		var body strings.Builder
		for scan.Scan() {
			line := scan.Text()
			if data {
				if line == "." {
					data = false
					received <- body.String()
					fmt.Fprint(conn, "250 queued\r\n")
				} else {
					body.WriteString(line + "\n")
				}
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				fmt.Fprint(conn, "250-fixture\r\n250 8BITMIME\r\n")
			case strings.HasPrefix(line, "DATA"):
				data = true
				fmt.Fprint(conn, "354 continue\r\n")
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 OK\r\n")
			}
		}
	}()
	f := newManagementFixture(t, NewMemoryRepository())
	c := defaultSMTPConfig()
	c.Enabled = true
	c.Host = "127.0.0.1"
	c.Port = listener.Addr().(*net.TCPAddr).Port
	c.Security = "local"
	c.From = "sender@example.test"
	c.BaseURL = "https://watch.example.test"
	c.Password = "super-secret-mail-password"
	raw := f.call(t, "PUT", "/api/v1/admin/smtp", map[string]any{"version": 0, "config": c}, f.root.AccessToken, 200)
	if strings.Contains(string(raw), c.Password) || strings.Contains(string(raw), "v1.") {
		t.Fatal("SMTP response leaked credentials")
	}
	d, _ := f.repo.GetAdminDocument(context.Background(), "smtp")
	if strings.Contains(string(d.Data), c.Password) {
		t.Fatal("SMTP password unencrypted")
	}
	if e = NewRuntimeMailer(f.repo, f.server.options.Vault, nil).Send(context.Background(), MailMessage{Type: "verify_email", To: "recipient@example.test", Token: "safe-action-token", ExpiresAt: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, "safe-action-token") || !strings.Contains(body, "quoted-printable") {
			t.Fatalf("invalid message %s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SMTP message not received")
	}
	c.Host = "public.example.test"
	if validateSMTP(c) == nil {
		t.Fatal("plaintext SMTP allowed outside loopback")
	}
	c.Security = "starttls"
	c.From = "sender@example.test\r\nBcc: hidden@example.test"
	if validateSMTP(c) == nil {
		t.Fatal("mail header injection")
	}
}
func TestEmailCodeSingleUseAndMFA(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	mailer := &MemoryMailer{}
	f.auth.mailer = mailer
	c := defaultLoginConfig()
	c.EmailLogin = true
	f.call(t, "PUT", "/api/v1/admin/login-config", map[string]any{"version": 0, "config": c}, f.root.AccessToken, 200)
	var data struct {
		Challenge string `json:"challenge"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/auth/email/request", map[string]string{"email": f.member.User.Email}, "", 200), &data)
	mail, ok := mailer.LastMessage()
	if !ok || mail.Type != "login_code" {
		t.Fatal("email not sent")
	}
	f.call(t, "POST", "/api/v1/auth/email/finish", map[string]string{"challenge": data.Challenge, "code": mail.Token}, "", 200)
	f.call(t, "POST", "/api/v1/auth/email/finish", map[string]string{"challenge": data.Challenge, "code": mail.Token}, "", 401)
	json.Unmarshal(f.call(t, "POST", "/api/v1/auth/email/request", map[string]string{"email": f.member.User.Email}, "", 200), &data)
	f.call(t, "POST", "/api/v1/auth/email/finish", map[string]string{"challenge": data.Challenge, "code": "00000000"}, "", 401)
	mail, _ = mailer.LastMessage()
	f.call(t, "POST", "/api/v1/auth/email/finish", map[string]string{"challenge": data.Challenge, "code": mail.Token}, "", 401)
}
func TestOIDCRealCallbackAndExplicitLink(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	var issuer string
	var nonce string
	var challenge string
	var mu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "fixture", Algorithm: "RS256", Use: "sig"}}})
		case "/authorize":
			mu.Lock()
			nonce = r.URL.Query().Get("nonce")
			challenge = r.URL.Query().Get("code_challenge")
			mu.Unlock()
			u, _ := url.Parse(r.URL.Query().Get("redirect_uri"))
			q := u.Query()
			q.Set("state", r.URL.Query().Get("state"))
			q.Set("code", "fixture-code")
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), 302)
		case "/token":
			r.ParseForm()
			mu.Lock()
			n, ch := nonce, challenge
			mu.Unlock()
			if r.Form.Get("code_verifier") == "" || ch == "" {
				w.WriteHeader(400)
				return
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": issuer, "aud": "fixture-client", "sub": "fixture-subject", "email": "oidc@example.test", "email_verified": true, "name": "OIDC User", "nonce": n, "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix()})
			token.Header["kid"] = "fixture"
			signed, _ := token.SignedString(key)
			json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-access", "token_type": "Bearer", "id_token": signed})
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	issuer = upstream.URL
	f := newManagementFixture(t, NewMemoryRepository())
	c := defaultLoginConfig()
	c.BaseURL = f.http.URL
	c.Providers = []IdentityProvider{{ID: "fixture", Name: "Fixture", Kind: "oidc", Issuer: issuer, ClientID: "fixture-client", ClientSecret: "fixture-secret", Enabled: true}}
	raw := f.call(t, "PUT", "/api/v1/admin/login-config", map[string]any{"version": 0, "config": c}, f.root.AccessToken, 200)
	if strings.Contains(string(raw), "fixture-secret") {
		t.Fatal("OIDC secret leaked")
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", f.http.URL+"/api/v1/auth/oauth/fixture/start", strings.NewReader(`{}`))
	req.Header.Set("X-Device-ID", f.device.ID)
	req.Header.Set("Content-Type", "application/json")
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var start struct {
		Data struct {
			URL   string `json:"url"`
			Token string `json:"poll_token"`
		} `json:"data"`
	}
	json.NewDecoder(res.Body).Decode(&start)
	res.Body.Close()
	if start.Data.URL == "" {
		t.Fatal("missing authorization URL")
	}
	res, e = client.Get(start.Data.URL)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("OIDC callback %d", res.StatusCode)
	}
	raw = f.call(t, "POST", "/api/v1/auth/oauth/exchange", map[string]string{"token": start.Data.Token}, "", 200)
	var session Session
	json.Unmarshal(raw, &session)
	if session.User.Email != "oidc@example.test" || !session.User.EmailVerified {
		t.Fatalf("invalid OIDC user %+v", session.User)
	}
	// A stale or forged callback must never mint a login result.
	if _, e = f.repo.ConsumeAuthChallenge(context.Background(), "forged", "oauth-state"); !errors.Is(e, ErrInvalidActionToken) {
		t.Fatal(e)
	}
	if _, e = f.repo.ExternalIdentity(context.Background(), "oidc:"+issuer, "fixture-subject"); e != nil {
		t.Fatal(e)
	}
}
