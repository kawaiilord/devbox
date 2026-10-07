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
	accessLifetime  = 15 * time.Minute
	refreshLifetime = 30 * 24 * time.Hour
)

type accessClaims struct {
	DisplayName string `json:"name"`
	Email       string `json:"email"`
	jwt.RegisteredClaims
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

func (m *TokenManager) IssueAccess(user User) (string, error) {
	now := m.now()
	claims := accessClaims{
		DisplayName: user.DisplayName,
		Email:       user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.ID,
			Audience:  jwt.ClaimStrings{m.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessLifetime)),
			ID:        mustRandomString(12),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *TokenManager) ParseAccess(raw string) (User, error) {
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
		return User{}, ErrUnauthorized
	}
	return User{ID: claims.Subject, DisplayName: claims.DisplayName, Email: claims.Email}, nil
}

type AuthService struct {
	repository        Repository
	tokens            *TokenManager
	dummyPasswordHash string
	now               func() time.Time
}

func NewAuthService(repository Repository, tokens *TokenManager) *AuthService {
	dummyPasswordHash, err := hashPassword(mustRandomString(16))
	if err != nil {
		panic(err)
	}
	return &AuthService{
		repository: repository, tokens: tokens, dummyPasswordHash: dummyPasswordHash, now: time.Now,
	}
}

func (s *AuthService) Register(ctx context.Context, email, displayName, password string) (Session, error) {
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
			ID: mustRandomString(12), DisplayName: strings.TrimSpace(displayName), Email: normalizedEmail,
		},
		PasswordHash: passwordHash,
		CreatedAt:    s.now(),
	}
	if err := s.repository.CreateUser(ctx, account); err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, account.User)
}

func (s *AuthService) Login(ctx context.Context, email, password string) (Session, error) {
	account, err := s.repository.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		verifyPassword(password, s.dummyPasswordHash)
		return Session{}, ErrInvalidCredentials
	}
	if !verifyPassword(password, account.PasswordHash) {
		return Session{}, ErrInvalidCredentials
	}
	return s.issueSession(ctx, account.User)
}

func (s *AuthService) Refresh(ctx context.Context, rawRefresh string) (Session, error) {
	if len(rawRefresh) < 32 {
		return Session{}, ErrInvalidRefresh
	}
	newRefresh := mustRandomString(32)
	account, err := s.repository.RotateRefreshToken(
		ctx,
		hashRefreshToken(rawRefresh),
		hashRefreshToken(newRefresh),
		s.now().Add(refreshLifetime),
	)
	if err != nil {
		return Session{}, ErrInvalidRefresh
	}
	access, err := s.tokens.IssueAccess(account.User)
	if err != nil {
		return Session{}, err
	}
	return newSession(account.User, access, newRefresh), nil
}

func (s *AuthService) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	return s.repository.RevokeRefreshToken(ctx, hashRefreshToken(rawRefresh))
}

func (s *AuthService) CreateDemo(ctx context.Context, displayName string) (Session, error) {
	displayName = strings.TrimSpace(displayName)
	if len([]rune(displayName)) < 2 || len([]rune(displayName)) > 32 {
		return Session{}, errors.New("display name must be 2-32 characters")
	}
	id := mustRandomString(12)
	account := AccountRecord{
		User:         User{ID: id, DisplayName: displayName, Email: "demo-" + strings.ToLower(id) + "@invalid.local"},
		PasswordHash: "!demo-account",
		CreatedAt:    s.now(),
	}
	if err := s.repository.CreateUser(ctx, account); err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, account.User)
}

func (s *AuthService) AuthenticateAccess(ctx context.Context, raw string) (User, error) {
	user, err := s.tokens.ParseAccess(raw)
	if err != nil {
		return User{}, err
	}
	account, err := s.repository.UserByID(ctx, user.ID)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	return account.User, nil
}

func (s *AuthService) issueSession(ctx context.Context, user User) (Session, error) {
	access, err := s.tokens.IssueAccess(user)
	if err != nil {
		return Session{}, err
	}
	refresh := mustRandomString(32)
	if err := s.repository.StoreRefreshToken(
		ctx, user.ID, hashRefreshToken(refresh), s.now().Add(refreshLifetime),
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
