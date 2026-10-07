package app

import (
	"context"
	"errors"
	"testing"
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
	parsed, err := auth.AuthenticateAccess(ctx, registered.AccessToken)
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
