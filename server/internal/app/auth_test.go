package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRegisterLoginRefreshAndReplayProtection(t *testing.T) {
	repository := NewMemoryRepository()
	tokens, err := NewTokenManager("unit-test-secret-that-is-at-least-32-characters", "sameframe-test")
	if err != nil {
		t.Fatal(err)
	}
	auth := NewAuthService(repository, tokens)
	ctx := context.Background()

	registered, err := auth.Register(ctx, "Viewer@Example.com", "Viewer", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if registered.User.Email != "viewer@example.com" || registered.ExpiresIn != 900 {
		t.Fatalf("unexpected session: %+v", registered)
	}
	parsed, err := auth.AuthenticateAccess(ctx, registered.AccessToken, "test-device-0001")
	if err != nil || parsed.ID != registered.User.ID {
		t.Fatalf("parsed user=%+v error=%v", parsed, err)
	}

	loggedIn, err := auth.Login(ctx, "viewer@example.com", "correct horse battery")
	if err != nil || loggedIn.User.ID != registered.User.ID {
		t.Fatalf("login session=%+v error=%v", loggedIn, err)
	}
	if _, err = auth.Login(ctx, "viewer@example.com", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad password error=%v", err)
	}

	rotated, err := auth.Refresh(ctx, registered.RefreshToken)
	if err != nil || rotated.RefreshToken == registered.RefreshToken {
		t.Fatalf("rotated session=%+v error=%v", rotated, err)
	}
	if _, err = auth.Refresh(ctx, registered.RefreshToken); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("replayed refresh error=%v", err)
	}
	if err = auth.Logout(ctx, rotated.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Refresh(ctx, rotated.RefreshToken); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("logged-out refresh error=%v", err)
	}
}

func TestRegistrationRejectsDuplicateEmail(t *testing.T) {
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("unit-test-secret-that-is-at-least-32-characters", "sameframe-test")
	auth := NewAuthService(repository, tokens)
	ctx := context.Background()
	if _, err := auth.Register(ctx, "same@example.com", "First User", "a strong password"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Register(ctx, "SAME@example.com", "Second User", "another password"); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("duplicate registration error=%v", err)
	}
}

func TestEmailRecoveryAndDeviceRevocation(t *testing.T) {
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("security-test-secret-that-is-at-least-32-characters", "sameframe-security-test")
	mailer := &MemoryMailer{}
	auth := NewAuthService(repository, tokens, mailer)
	ctx := context.Background()
	deviceA := DeviceInfo{ID: "device-aaaaaaaa-0001", Label: "Laptop", Platform: "windows"}
	deviceB := DeviceInfo{ID: "device-bbbbbbbb-0002", Label: "Browser", Platform: "web"}

	registered, err := auth.Register(
		ctx, "secure@example.com", "Secure User", "original good password", deviceA,
	)
	if err != nil {
		t.Fatal(err)
	}
	if registered.User.EmailVerified {
		t.Fatal("new account unexpectedly verified")
	}
	verification, ok := mailer.LastMessage()
	if !ok || verification.Type != "verify_email" || verification.Token == "" {
		t.Fatalf("verification message=%+v present=%v", verification, ok)
	}
	if err := auth.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatal(err)
	}
	if err := auth.VerifyEmail(ctx, verification.Token); !errors.Is(err, ErrInvalidActionToken) {
		t.Fatalf("replayed verification error=%v", err)
	}

	secondSession, err := auth.Login(ctx, "secure@example.com", "original good password", deviceB)
	if err != nil || !secondSession.User.EmailVerified {
		t.Fatalf("second session=%+v error=%v", secondSession, err)
	}
	if _, err := auth.AuthenticateAccess(ctx, secondSession.AccessToken, deviceA.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-device access error=%v", err)
	}
	devices, err := repository.UserDevices(ctx, registered.User.ID)
	if err != nil || len(devices) != 2 {
		t.Fatalf("devices=%+v error=%v", devices, err)
	}
	_, deviceBHash, _ := normalizeDevice(deviceB)
	if err := repository.RevokeUserDevice(ctx, registered.User.ID, deviceBHash); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.AuthenticateAccess(ctx, secondSession.AccessToken, deviceB.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked-device access error=%v", err)
	}
	if _, err := auth.Login(ctx, "secure@example.com", "original good password", deviceB); !errors.Is(err, ErrDeviceBlocked) {
		t.Fatalf("revoked-device login error=%v", err)
	}

	auth.RequestPasswordReset(ctx, "secure@example.com")
	var reset MailMessage
	for attempt := 0; attempt < 50; attempt++ {
		reset, ok = mailer.LastMessage()
		if ok && reset.Type == "reset_password" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if reset.Type != "reset_password" {
		t.Fatal("password reset message was not queued")
	}
	if err := auth.ResetPassword(ctx, reset.Token, "replacement good password"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.AuthenticateAccess(ctx, registered.AccessToken, deviceA.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("pre-reset access token error=%v", err)
	}
	if err := auth.ResetPassword(ctx, reset.Token, "another replacement password"); !errors.Is(err, ErrInvalidActionToken) {
		t.Fatalf("replayed reset error=%v", err)
	}
	if _, err := auth.Login(ctx, "secure@example.com", "original good password", deviceA); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password login error=%v", err)
	}
	if _, err := auth.Login(ctx, "secure@example.com", "replacement good password", deviceA); err != nil {
		t.Fatalf("replacement password login error=%v", err)
	}
	if _, err := auth.Refresh(ctx, registered.RefreshToken, deviceA); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("pre-reset refresh token error=%v", err)
	}
}
