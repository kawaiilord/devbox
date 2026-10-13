package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebEntryCookieIsBoundAndSingleUse(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	request := func(path, token, device string, cookie *http.Cookie, payload any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Device-ID", device)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		f.server.Handler().ServeHTTP(res, req)
		return res
	}
	mint := func() *http.Cookie {
		t.Helper()
		res := request(webEntryPath, f.member.AccessToken, f.device.ID, nil, map[string]string{})
		if res.Code != 200 {
			t.Fatalf("mint: %d %s", res.Code, res.Body)
		}
		cookies := res.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("missing handoff cookie")
		}
		c := cookies[0]
		if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != webEntryPath || c.MaxAge != 120 || c.Domain != "" {
			t.Fatalf("cookie scope/flags: %+v", c)
		}
		if bytes.Contains(res.Body.Bytes(), []byte(c.Value)) {
			t.Fatal("handoff secret returned to JavaScript")
		}
		return c
	}
	if r := request(webEntryPath, "", f.device.ID, nil, map[string]string{}); r.Code != 401 {
		t.Fatal("anonymous handoff allowed")
	}
	if r := request(webEntryPath+"/exchange", "", f.device.ID, nil, map[string]string{}); r.Code != 200 {
		t.Fatal("no-cookie boot must permit existing secure storage restoration")
	}
	cookie := mint()
	crossOrigin := httptest.NewRequest("POST", webEntryPath+"/exchange", bytes.NewReader([]byte("{}")))
	crossOrigin.Header.Set("Origin", "https://another.example.test")
	crossOrigin.Header.Set("X-Device-ID", f.device.ID)
	crossOrigin.AddCookie(cookie)
	rejected := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(rejected, crossOrigin)
	if rejected.Code != 403 {
		t.Fatal("cross-origin handoff was not rejected")
	}
	res := request(webEntryPath+"/exchange", "", f.device.ID, cookie, map[string]string{})
	if res.Code != 200 {
		t.Fatalf("exchange: %d %s", res.Code, res.Body)
	}
	var reply struct {
		Data Session `json:"data"`
	}
	json.Unmarshal(res.Body.Bytes(), &reply)
	if reply.Data.User.ID != f.member.User.ID || reply.Data.AccessToken == "" || reply.Data.RefreshToken == "" {
		t.Fatal("wrong or missing transferred account")
	}
	if r := request(webEntryPath+"/exchange", "", f.device.ID, cookie, map[string]string{}); r.Code != 401 {
		t.Fatal("handoff replay accepted")
	}
	cookie = mint()
	if r := request(webEntryPath+"/exchange", "", "different-device-0001", cookie, map[string]string{}); r.Code != 401 {
		t.Fatal("cross-device handoff accepted")
	}
	// Expiration and account revocation are checked after consuming the proof.
	expired := mustRandomString(32)
	raw, _ := json.Marshal(webEntryProof{UserID: f.member.User.ID, DeviceHash: hashDeviceID(f.device.ID), Version: 1})
	f.repo.SaveAuthChallenge(context.Background(), expired, AuthChallenge{Purpose: "web-entry", Data: raw, ExpiresAt: time.Now().Add(-time.Second)})
	if r := request(webEntryPath+"/exchange", "", f.device.ID, entryCookie(expired, 120), map[string]string{}); r.Code != 401 {
		t.Fatal("expired handoff accepted")
	}
	cookie = mint()
	f.repo.RevokeUserDevice(context.Background(), f.member.User.ID, hashDeviceID(f.device.ID))
	if r := request(webEntryPath+"/exchange", "", f.device.ID, cookie, map[string]string{}); r.Code != 401 {
		t.Fatal("revoked device handoff accepted")
	}
}

func TestRegistrationEmailSwitchOverridesLegacySetting(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	mailer := &MemoryMailer{}
	f.auth.mailer = mailer
	set := func(required bool) {
		p, d, err := f.server.sitePolicy(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		p.RegistrationRequiresEmailCode = required
		f.call(t, "PUT", "/api/v1/admin/site-policy", map[string]any{"version": d.Version, "policy": p}, f.root.AccessToken, 200)
	}
	rules := func(expected bool) {
		var out map[string]any
		json.Unmarshal(f.call(t, "GET", "/api/v1/auth/options", nil, "", 200), &out)
		if out["registration_email_code_required"] != expected {
			t.Fatal("published registration rule differs from the admin switch")
		}
	}
	set(true)
	rules(true)
	payload := map[string]string{"email": "switch-on@example.test", "display_name": "Switch on", "password": "a secure test password"}
	f.call(t, "POST", "/api/v1/auth/register", payload, "", 400)
	var proof struct {
		Challenge string `json:"challenge"`
	}
	json.Unmarshal(f.call(t, "POST", "/api/v1/auth/register/code", map[string]string{"email": payload["email"]}, "", 200), &proof)
	message, ok := mailer.LastMessage()
	if !ok || message.Type != "register_code" {
		t.Fatal("no registration code")
	}
	payload["challenge"], payload["code"] = proof.Challenge, message.Token
	var account Session
	json.Unmarshal(f.call(t, "POST", "/api/v1/auth/register", payload, "", 201), &account)
	if !account.User.EmailVerified {
		t.Fatal("verified registration not marked verified")
	}
	// An explicit admin OFF takes precedence even over the old environment option.
	f.server.options.RequireRegistrationCode = true
	set(false)
	rules(false)
	f.call(t, "POST", "/api/v1/auth/register/code", map[string]string{"email": "disabled@example.test"}, "", 403)
	payload = map[string]string{"email": "switch-off@example.test", "display_name": "Switch off", "password": "a secure test password"}
	json.Unmarshal(f.call(t, "POST", "/api/v1/auth/register", payload, "", 201), &account)
	if account.User.EmailVerified {
		t.Fatal("disabled verification incorrectly certifies email")
	}
	// Invitation and email verification remain independent settings.
	p, d, _ := f.server.sitePolicy(context.Background())
	p.RegistrationRequiresInvite = true
	f.call(t, "PUT", "/api/v1/admin/site-policy", map[string]any{"version": d.Version, "policy": p}, f.root.AccessToken, 200)
	f.call(t, "POST", "/api/v1/auth/register", map[string]string{"email": "no-invite@example.test", "display_name": "No invite", "password": "a secure test password"}, "", 400)
}
