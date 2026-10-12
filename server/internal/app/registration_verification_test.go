package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVerifiedRegistrationAndAccountBoundary(t *testing.T) {
	repo := NewMemoryRepository()
	tokens, _ := NewTokenManager("registration-test-secret-at-least-32-bytes", "verified-registration")
	mailer := &MemoryMailer{}
	server := NewServer(Options{Repository: repo, Auth: NewAuthService(repo, tokens, mailer), RequireAccount: true, RequireRegistrationCode: true, AllowDemoAuth: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	device := DeviceInfo{ID: "registration-device-0001", Label: "Test", Platform: "web"}
	email := "verified@example.test"
	payload := map[string]string{"email": email, "display_name": "Verified", "password": "a strong registration password"}
	if got := requestStatusDevice(t, "POST", httpServer.URL+"/api/v1/auth/register", "", payload, device); got != 400 {
		t.Fatalf("registration without code: %d", got)
	}
	if _, err := repo.UserByEmail(context.Background(), email); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("account created without email proof")
	}
	for _, path := range []string{"/api/v1/public/rooms", "/api/v1/rooms/ABCDEF", "/api/v1/rooms/ABCDEF/messages"} {
		if got := requestStatusDevice(t, "GET", httpServer.URL+path, "", nil, device); got != 401 {
			t.Fatalf("anonymous %s: %d", path, got)
		}
	}
	for _, path := range []string{"/api/v1/rooms/ABCDEF/guest", "/api/v1/session/demo"} {
		got := requestStatusDevice(t, "POST", httpServer.URL+path, "", map[string]string{"display_name": "Guest"}, device)
		if got < 400 {
			t.Fatalf("anonymous session bypass at %s", path)
		}
	}
	raw := requestJSONDevice(t, "POST", httpServer.URL+"/api/v1/auth/register/code", "", map[string]string{"email": email}, device)
	var response struct {
		Data struct {
			Challenge string `json:"challenge"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	message, ok := mailer.LastMessage()
	if !ok || message.Type != "register_code" || len(message.Token) != 6 || strings.Contains(string(raw), message.Token) {
		t.Fatal("code delivery or response secrecy failed")
	}
	payload["challenge"], payload["code"] = response.Data.Challenge, message.Token
	raw = requestJSONDevice(t, "POST", httpServer.URL+"/api/v1/auth/register", "", payload, device)
	var registered struct {
		Data Session `json:"data"`
	}
	json.Unmarshal(raw, &registered)
	if registered.Data.AccessToken == "" || !registered.Data.User.EmailVerified {
		t.Fatal("verified registration did not issue a verified account")
	}
	if got := requestStatusDevice(t, "GET", httpServer.URL+"/api/v1/public/rooms", registered.Data.AccessToken, nil, device); got != 200 {
		t.Fatalf("authenticated discovery: %d", got)
	}
	if got := requestStatusDevice(t, "POST", httpServer.URL+"/api/v1/auth/register", "", payload, device); got != 400 {
		t.Fatalf("verification replay accepted: %d", got)
	}
	// A valid legacy guest/demo token must not bypass the new account boundary.
	demo, err := server.auth.CreateDemo(context.Background(), "Legacy", device)
	if err != nil {
		t.Fatal(err)
	}
	if got := requestStatusDevice(t, "GET", httpServer.URL+"/api/v1/public/rooms", demo.AccessToken, nil, device); got != 401 {
		t.Fatalf("legacy anonymous token accepted: %d", got)
	}
}

func TestRegistrationFailsClosedWithoutMail(t *testing.T) {
	server := NewServer(Options{RequireAccount: true, RequireRegistrationCode: true})
	h := httptest.NewServer(server.Handler())
	defer h.Close()
	device := DeviceInfo{ID: "no-mail-device-0001", Label: "Test", Platform: "web"}
	if got := requestStatusDevice(t, "POST", h.URL+"/api/v1/auth/register/code", "", map[string]string{"email": "no-mail@example.test"}, device); got != 503 {
		t.Fatalf("unconfigured mail: %d", got)
	}
	if _, err := server.repo.UserByEmail(context.Background(), "no-mail@example.test"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("failed delivery created an account")
	}
}

func TestRegistrationProofBindingExpiryAndSingleUse(t *testing.T) {
	repo := NewMemoryRepository()
	server := NewServer(Options{Repository: repo})
	ctx := context.Background()
	for _, mode := range []string{"email", "device", "code", "expired"} {
		challenge := mustRandomString(32)
		data, _ := json.Marshal(registrationVerification{Email: "proof@example.test", DeviceHash: hashDeviceID("proof-device"), CodeHash: hashRefreshToken(challenge + ":123456")})
		expiry := time.Now().Add(time.Minute)
		if mode == "expired" {
			expiry = time.Now().Add(-time.Second)
		}
		if err := repo.SaveAuthChallenge(ctx, challenge, AuthChallenge{Purpose: "registration", Data: data, ExpiresAt: expiry}); err != nil {
			t.Fatal(err)
		}
		email, device, code := "proof@example.test", "proof-device", "123456"
		if mode == "email" {
			email = "other@example.test"
		}
		if mode == "device" {
			device = "other-device"
		}
		if mode == "code" {
			code = "654321"
		}
		if server.verifyRegistrationCode(ctx, email, device, challenge, code) == nil {
			t.Fatalf("accepted mismatched/expired proof: %s", mode)
		}
		if server.verifyRegistrationCode(ctx, "proof@example.test", "proof-device", challenge, "123456") == nil {
			t.Fatalf("reused consumed proof: %s", mode)
		}
	}
	challenge := mustRandomString(32)
	data, _ := json.Marshal(registrationVerification{Email: "proof@example.test", DeviceHash: hashDeviceID("proof-device"), CodeHash: hashRefreshToken(challenge + ":123456")})
	if err := repo.SaveAuthChallenge(ctx, challenge, AuthChallenge{Purpose: "registration", Data: data, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- server.verifyRegistrationCode(ctx, "proof@example.test", "proof-device", challenge, "123456") == nil
		}()
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
		t.Fatalf("proof accepted %d times", count)
	}
}
