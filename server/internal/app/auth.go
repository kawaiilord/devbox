package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

const (
	accessLifetime        = 15 * time.Minute
	refreshLifetime       = 30 * 24 * time.Hour
	verificationLifetime  = 24 * time.Hour
	passwordResetLifetime = 30 * time.Minute
)

type accessClaims struct {
	GuestRoomCode  string `json:"grc,omitempty"`
	DisplayName    string `json:"name"`
	Email          string `json:"email"`
	DeviceHash     string `json:"did"`
	SessionVersion int64  `json:"ver"`
	jwt.RegisteredClaims
}

type AccessIdentity struct {
	User       User
	DeviceHash string
}

type TokenManager struct {
	secret   []byte
	issuer   string
	audience string
	now      func() time.Time
}

func NewTokenManager(secret, issuer string) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 characters")
	}
	if issuer == "" {
		issuer = "sameframe"
	}
	return &TokenManager{
		secret: []byte(secret), issuer: issuer, audience: "sameframe-client", now: time.Now,
	}, nil
}

func (m *TokenManager) IssueAccess(user User, deviceHash string) (string, error) {
	return m.issueAccessUntil(user, deviceHash, m.now().Add(accessLifetime))
}

func (m *TokenManager) issueAccessUntil(user User, deviceHash string, expires time.Time) (string, error) {
	now := m.now()
	claims := accessClaims{
		GuestRoomCode:  user.GuestRoomCode,
		DisplayName:    user.DisplayName,
		Email:          user.Email,
		DeviceHash:     deviceHash,
		SessionVersion: user.SessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.ID,
			Audience:  jwt.ClaimStrings{m.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(expires),
			ID:        mustRandomString(12),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *TokenManager) ParseAccess(raw string) (AccessIdentity, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil || !token.Valid || claims.Subject == "" {
		return AccessIdentity{}, ErrUnauthorized
	}
	if claims.DeviceHash == "" || claims.SessionVersion < 1 {
		return AccessIdentity{}, ErrUnauthorized
	}
	return AccessIdentity{
		User: User{
			GuestRoomCode:  claims.GuestRoomCode,
			GuestExpiresAt: claims.ExpiresAt.Time.UnixMilli(),
			ID:             claims.Subject, DisplayName: claims.DisplayName, Email: claims.Email,
			SessionVersion: claims.SessionVersion,
		},
		DeviceHash: claims.DeviceHash,
	}, nil
}

type AuthService struct {
	repository        Repository
	tokens            *TokenManager
	mailer            Mailer
	adminEmails       map[string]struct{}
	dummyPasswordHash string
	now               func() time.Time
}

func NewAuthService(repository Repository, tokens *TokenManager, mailers ...Mailer) *AuthService {
	dummyPasswordHash, err := hashPassword(mustRandomString(16))
	if err != nil {
		panic(err)
	}
	var mailer Mailer = NoopMailer{}
	if len(mailers) > 0 && mailers[0] != nil {
		mailer = mailers[0]
	}
	return &AuthService{
		repository: repository, tokens: tokens, mailer: mailer,
		adminEmails: make(map[string]struct{}), dummyPasswordHash: dummyPasswordHash, now: time.Now,
	}
}

func (s *AuthService) SetAdminEmails(emails []string) {
	s.adminEmails = make(map[string]struct{}, len(emails))
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			s.adminEmails[email] = struct{}{}
		}
	}
}

func (s *AuthService) Register(
	ctx context.Context,
	email, displayName, password string,
	devices ...DeviceInfo,
) (Session, error) {
	device, deviceHash, err := normalizeDevice(firstDevice(devices))
	if err != nil {
		return Session{}, err
	}
	normalizedEmail, err := validateCredentials(email, displayName, password)
	if err != nil {
		return Session{}, err
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return Session{}, err
	}
	account := AccountRecord{
		User: User{
			ID: mustRandomString(12), DisplayName: strings.TrimSpace(displayName),
			Email: normalizedEmail, SessionVersion: 1,
		},
		PasswordHash: passwordHash,
		CreatedAt:    s.now(),
	}
	_, account.IsAdmin = s.adminEmails[normalizedEmail]
	if err := s.repository.CreateUser(ctx, account); err != nil {
		return Session{}, err
	}
	if err := s.repository.BindDevice(ctx, account.ID, deviceHash, device); err != nil {
		return Session{}, err
	}
	if err := s.issueActionToken(ctx, account.User, "verify_email", verificationLifetime); err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, account.User, deviceHash)
}

func (s *AuthService) Login(
	ctx context.Context,
	email, password string,
	devices ...DeviceInfo,
) (Session, error) {
	device, deviceHash, err := normalizeDevice(firstDevice(devices))
	if err != nil {
		return Session{}, err
	}
	account, err := s.repository.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		verifyPassword(password, s.dummyPasswordHash)
		return Session{}, ErrInvalidCredentials
	}
	if !verifyPassword(password, account.PasswordHash) {
		return Session{}, ErrInvalidCredentials
	}
	if err := s.repository.BindDevice(ctx, account.ID, deviceHash, device); err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, account.User, deviceHash)
}

func (s *AuthService) Refresh(
	ctx context.Context,
	rawRefresh string,
	devices ...DeviceInfo,
) (Session, error) {
	_, deviceHash, err := normalizeDevice(firstDevice(devices))
	if err != nil {
		return Session{}, ErrInvalidRefresh
	}
	if len(rawRefresh) < 32 {
		return Session{}, ErrInvalidRefresh
	}
	newRefresh := mustRandomString(32)
	identity, err := s.repository.RotateRefreshToken(
		ctx,
		hashRefreshToken(rawRefresh),
		deviceHash,
		hashRefreshToken(newRefresh),
		s.now().Add(refreshLifetime),
	)
	if err != nil {
		return Session{}, ErrInvalidRefresh
	}
	if err := s.repository.ValidateDevice(ctx, identity.Account.ID, identity.DeviceHash); err != nil {
		return Session{}, ErrInvalidRefresh
	}
	access, err := s.tokens.IssueAccess(identity.Account.User, identity.DeviceHash)
	if err != nil {
		return Session{}, err
	}
	return newSession(identity.Account.User, access, newRefresh), nil
}

func (s *AuthService) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	return s.repository.RevokeRefreshToken(ctx, hashRefreshToken(rawRefresh))
}

func (s *AuthService) CreateDemo(ctx context.Context, displayName string, devices ...DeviceInfo) (Session, error) {
	device, deviceHash, err := normalizeDevice(firstDevice(devices))
	if err != nil {
		return Session{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if len([]rune(displayName)) < 2 || len([]rune(displayName)) > 32 {
		return Session{}, errors.New("display name must be 2-32 characters")
	}
	id := mustRandomString(12)
	account := AccountRecord{
		User: User{
			ID: id, DisplayName: displayName, Email: "demo-" + strings.ToLower(id) + "@invalid.local",
			EmailVerified: true, SessionVersion: 1,
		},
		PasswordHash: "!demo-account",
		CreatedAt:    s.now(),
	}
	if err := s.repository.CreateUser(ctx, account); err != nil {
		return Session{}, err
	}
	if err := s.repository.BindDevice(ctx, account.ID, deviceHash, device); err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, account.User, deviceHash)
}

func (s *AuthService) AuthenticateAccess(ctx context.Context, raw, rawDeviceID string) (User, error) {
	identity, err := s.tokens.ParseAccess(raw)
	if err != nil {
		return User{}, err
	}
	if hashDeviceID(rawDeviceID) != identity.DeviceHash {
		return User{}, ErrUnauthorized
	}
	account, err := s.repository.UserByID(ctx, identity.User.ID)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	if err := s.repository.ValidateDevice(ctx, account.ID, identity.DeviceHash); err != nil {
		return User{}, ErrUnauthorized
	}
	if account.SessionVersion != identity.User.SessionVersion {
		return User{}, ErrUnauthorized
	}
	account.User.GuestRoomCode = identity.User.GuestRoomCode
	if account.User.GuestRoomCode != "" {
		account.User.GuestExpiresAt = identity.User.GuestExpiresAt
		account.User.Email = ""
	}
	return account.User, nil
}

func (s *AuthService) issueSession(ctx context.Context, user User, deviceHash string) (Session, error) {
	access, err := s.tokens.IssueAccess(user, deviceHash)
	if err != nil {
		return Session{}, err
	}
	refresh := mustRandomString(32)
	if err := s.repository.StoreRefreshToken(
		ctx, user.ID, deviceHash, hashRefreshToken(refresh), s.now().Add(refreshLifetime),
	); err != nil {
		return Session{}, err
	}
	return newSession(user, access, refresh), nil
}

func newSession(user User, access, refresh string) Session {
	return Session{
		AccessToken: access, RefreshToken: refresh, TokenType: "bearer",
		ExpiresIn: int64(accessLifetime.Seconds()), User: user,
	}
}

func (s *AuthService) RequestEmailVerification(ctx context.Context, user User) error {
	account, err := s.repository.UserByID(ctx, user.ID)
	if err != nil {
		return err
	}
	if account.EmailVerified {
		return nil
	}
	return s.issueActionToken(ctx, account.User, "verify_email", verificationLifetime)
}

func (s *AuthService) VerifyEmail(ctx context.Context, rawToken string) error {
	account, err := s.repository.ConsumeActionToken(
		ctx, hashRefreshToken(rawToken), "verify_email",
	)
	if err != nil {
		return ErrInvalidActionToken
	}
	return s.repository.VerifyEmail(ctx, account.ID)
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) {
	email = strings.ToLower(strings.TrimSpace(email))
	account, err := s.repository.UserByEmail(ctx, email)
	verifyPassword(mustRandomString(16), s.dummyPasswordHash)
	if err != nil {
		return
	}
	go func() {
		mailCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = s.issueActionToken(mailCtx, account.User, "reset_password", passwordResetLifetime)
	}()
}

func (s *AuthService) ResetPassword(ctx context.Context, rawToken, password string) error {
	if len(password) < 10 || len(password) > 128 {
		return errors.New("password must be 10-128 characters")
	}
	account, err := s.repository.ConsumeActionToken(
		ctx, hashRefreshToken(rawToken), "reset_password",
	)
	if err != nil {
		return ErrInvalidActionToken
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if err := s.repository.UpdatePassword(ctx, account.ID, passwordHash); err != nil {
		return err
	}
	return s.repository.RevokeUserRefreshTokens(ctx, account.ID)
}

func (s *AuthService) issueActionToken(
	ctx context.Context,
	user User,
	purpose string,
	lifetime time.Duration,
) error {
	raw := mustRandomString(32)
	expiresAt := s.now().Add(lifetime)
	if err := s.repository.StoreActionToken(ctx, ActionTokenRecord{
		Hash: hashRefreshToken(raw), UserID: user.ID, Purpose: purpose, ExpiresAt: expiresAt,
	}); err != nil {
		return err
	}
	return s.mailer.Send(ctx, MailMessage{
		Type: purpose, To: user.Email, Token: raw, ExpiresAt: expiresAt,
	})
}

func firstDevice(devices []DeviceInfo) DeviceInfo {
	if len(devices) > 0 {
		return devices[0]
	}
	return DeviceInfo{ID: "test-device-0001", Label: "Test device", Platform: "test"}
}

func normalizeDevice(device DeviceInfo) (DeviceInfo, string, error) {
	device.ID = strings.TrimSpace(device.ID)
	device.Label = strings.TrimSpace(device.Label)
	device.Platform = strings.ToLower(strings.TrimSpace(device.Platform))
	if len(device.ID) < 16 || len(device.ID) > 128 {
		return DeviceInfo{}, "", errors.New("invalid device identifier")
	}
	if device.Label == "" {
		device.Label = "Unknown device"
	}
	if len([]rune(device.Label)) > 64 {
		return DeviceInfo{}, "", errors.New("device label is too long")
	}
	if device.Platform == "" {
		device.Platform = "unknown"
	}
	if len(device.Platform) > 32 {
		return DeviceInfo{}, "", errors.New("invalid device platform")
	}
	return device, hashDeviceID(device.ID), nil
}

func hashDeviceID(raw string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(digest[:])
}

func validateCredentials(email, displayName, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return "", errors.New("invalid email address")
	}
	nameLength := len([]rune(strings.TrimSpace(displayName)))
	if nameLength < 2 || nameLength > 32 {
		return "", errors.New("display name must be 2-32 characters")
	}
	if len(password) < 10 || len(password) > 128 {
		return "", errors.New("password must be 10-128 characters")
	}
	return email, nil
}

func hashPassword(password string) (string, error) {
	const (
		memory      = 64 * 1024
		iterations  = 2
		parallelism = 2
		keyLength   = 32
	)
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version, memory, iterations, parallelism int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(
		parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism,
	); err != nil || version != argon2.Version || memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 || len(expected) > 64 {
		return false
	}
	actual := argon2.IDKey(
		[]byte(password), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func hashRefreshToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func mustRandomString(size int) string {
	value, err := randomString(size)
	if err != nil {
		panic(err)
	}
	return value
}
