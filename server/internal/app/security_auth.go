package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type IdentityProvider struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Issuer         string `json:"issuer"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret,omitempty"`
	SecretEnvelope string `json:"secret_envelope,omitempty"`
	HasSecret      bool   `json:"has_secret"`
	Enabled        bool   `json:"enabled"`
}
type LoginConfig struct {
	PasswordLogin  bool               `json:"password_login"`
	EmailLogin     bool               `json:"email_login"`
	TOTPEnrollment bool               `json:"totp_enrollment"`
	Passkeys       bool               `json:"passkeys"`
	BaseURL        string             `json:"base_url"`
	RPDisplayName  string             `json:"rp_display_name"`
	Providers      []IdentityProvider `json:"providers"`
}

func defaultLoginConfig() LoginConfig {
	return LoginConfig{PasswordLogin: true, TOTPEnrollment: true, RPDisplayName: "同帧", Providers: []IdentityProvider{}}
}
func readLoginConfig(ctx context.Context, r Repository) (LoginConfig, int64, error) {
	c := defaultLoginConfig()
	v, e := readServiceConfig(ctx, r, "login", &c)
	return c, v, e
}

type LoginChallengeError struct{ Token string }

func (e *LoginChallengeError) Error() string {
	return "请输入身份验证器的动态验证码或恢复码"
}
func writeLoginChallenge(w http.ResponseWriter, err error) bool {
	var c *LoginChallengeError
	if !errors.As(err, &c) {
		return false
	}
	writeJSON(w, 202, apiResponse{Code: 202, Msg: c.Error(), Data: map[string]any{"mfa_required": true, "challenge": c.Token}})
	return true
}

type loginCeremony struct {
	UserID          string `json:"user_id"`
	SessionVersion  int64  `json:"session_version"`
	DeviceHash      string `json:"device_hash"`
	Secret          string `json:"secret,omitempty"`
	CodeHash        string `json:"code_hash,omitempty"`
	SecurityVersion int64  `json:"security_version"`
}

func saveChallenge(ctx context.Context, repo Repository, purpose string, data any) (string, error) {
	raw, e := json.Marshal(data)
	if e != nil {
		return "", e
	}
	token := mustRandomString(32)
	return token, repo.SaveAuthChallenge(ctx, token, AuthChallenge{Purpose: purpose, Data: raw, ExpiresAt: time.Now().Add(5 * time.Minute)})
}
func consumeChallenge(ctx context.Context, repo Repository, token, purpose string, out any) error {
	if len(token) < 32 || len(token) > 256 {
		return ErrInvalidActionToken
	}
	v, e := repo.ConsumeAuthChallenge(ctx, token, purpose)
	if e != nil {
		return e
	}
	return json.Unmarshal(v.Data, out)
}
func (s *AuthService) beginAuthenticatedLogin(ctx context.Context, user User, deviceHash string) (Session, error) {
	state, e := s.repository.ReadAccountSecurity(ctx, user.ID)
	if e != nil {
		return Session{}, e
	}
	if state.TOTPSecret != "" {
		token, e := saveChallenge(ctx, s.repository, "mfa", loginCeremony{UserID: user.ID, SessionVersion: user.SessionVersion, DeviceHash: deviceHash, SecurityVersion: state.Version})
		if e != nil {
			return Session{}, e
		}
		return Session{}, &LoginChallengeError{Token: token}
	}
	return s.issueSession(ctx, user, deviceHash)
}
func totpAt(secret string, step int64) string {
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if e != nil {
		return ""
	}
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], uint64(step))
	h := hmac.New(sha1.New, key)
	h.Write(raw[:])
	b := h.Sum(nil)
	offset := b[len(b)-1] & 15
	code := (binary.BigEndian.Uint32(b[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code)
}
func verifyTOTP(secret, code string, after int64) (int64, bool) {
	step := time.Now().Unix() / 30
	for _, v := range []int64{step, step - 1, step + 1} {
		if v > after && subtle.ConstantTimeCompare([]byte(totpAt(secret, v)), []byte(code)) == 1 {
			return v, true
		}
	}
	return 0, false
}
func (s *Server) checkFactor(state *AccountSecurity, userID, code string) error {
	if state.TOTPSecret == "" || s.options.Vault == nil {
		return ErrInvalidCredentials
	}
	plain, e := s.options.Vault.Decrypt(state.TOTPSecret, "totp:"+userID)
	if e != nil {
		return ErrInvalidCredentials
	}
	if step, ok := verifyTOTP(string(plain), code, state.LastStep); ok {
		state.LastStep = step
		return nil
	}
	hash := hashRefreshToken(strings.ToUpper(strings.TrimSpace(code)))
	for i, v := range state.RecoveryHashes {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(v)) == 1 {
			state.RecoveryHashes = append(state.RecoveryHashes[:i:i], state.RecoveryHashes[i+1:]...)
			return nil
		}
	}
	return ErrInvalidCredentials
}
func (s *Server) finishMFA(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "mfa", sourceIdentity(r), 15, 15*time.Minute) {
		return
	}
	var in struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	var c loginCeremony
	e := consumeChallenge(r.Context(), s.repo, in.Challenge, "mfa", &c)
	if e != nil || c.DeviceHash != hashDeviceID(r.Header.Get("X-Device-ID")) {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	a, e := s.repo.UserByID(r.Context(), c.UserID)
	if e != nil || !accountActive(a.User) || a.SessionVersion != c.SessionVersion {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	state, e := s.repo.ReadAccountSecurity(r.Context(), a.ID)
	if e != nil || state.Version != c.SecurityVersion {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	if e = s.checkFactor(&state, a.ID, in.Code); e != nil {
		writeError(w, 401, errors.New("验证码无效或已使用，请重新登录"))
		return
	}
	if _, e = s.repo.WriteAccountSecurity(r.Context(), a.ID, a.SessionVersion, state, false); e != nil {
		writeAdminError(w, e)
		return
	}
	session, e := s.auth.issueSession(r.Context(), a.User, c.DeviceHash)
	if e != nil {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
func (s *Server) accountSecurity(w http.ResponseWriter, r *http.Request) {
	user, e := s.userFromRequest(r)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	state, e := s.repo.ReadAccountSecurity(r.Context(), user.ID)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	keys := []map[string]any{}
	for _, k := range state.Passkeys {
		keys = append(keys, map[string]any{"id": credentialID(k.Credential.ID), "name": k.Name, "created_at": k.CreatedAt})
	}
	c, _, e := readLoginConfig(r.Context(), s.repo)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"totp_enabled": state.TOTPSecret != "", "totp_enrollment": c.TOTPEnrollment, "passkeys_enabled": c.Passkeys, "passkeys": keys, "recovery_codes_remaining": len(state.RecoveryHashes)}})
}

type securityInput struct {
	Password  string `json:"password"`
	Code      string `json:"code"`
	Challenge string `json:"challenge"`
	Name      string `json:"name"`
}

func (s *Server) reauthenticate(ctx context.Context, user User, in securityInput) (AccountRecord, AccountSecurity, error) {
	a, e := s.repo.UserByID(ctx, user.ID)
	if e != nil || a.SessionVersion != user.SessionVersion || !accountActive(a.User) || !verifyPassword(in.Password, a.PasswordHash) {
		return a, AccountSecurity{}, ErrInvalidCredentials
	}
	state, e := s.repo.ReadAccountSecurity(ctx, user.ID)
	if e == nil && state.TOTPSecret != "" {
		e = s.checkFactor(&state, user.ID, in.Code)
	}
	return a, state, e
}
func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	user, e := s.userFromRequest(r)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	if !s.enforceRateLimit(w, r, "security", user.ID, 10, 15*time.Minute) {
		return
	}
	c, _, e := readLoginConfig(r.Context(), s.repo)
	if e != nil || !c.TOTPEnrollment || s.options.Vault == nil {
		writeError(w, 503, errors.New("未启用动态验证码配置或密钥库"))
		return
	}
	var in securityInput
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	a, state, e := s.reauthenticate(r.Context(), user, in)
	if e != nil {
		writeError(w, 403, ErrInvalidCredentials)
		return
	}
	if state.TOTPSecret != "" {
		writeError(w, 409, errors.New("已启用动态验证码"))
		return
	}
	key := make([]byte, 20)
	if _, e = rand.Read(key); e != nil {
		writeError(w, 500, e)
		return
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key)
	sealed, e := s.options.Vault.Encrypt([]byte(secret), "totp:"+user.ID)
	if e != nil {
		writeError(w, 500, e)
		return
	}
	token, e := saveChallenge(r.Context(), s.repo, "totp-setup", loginCeremony{UserID: user.ID, SessionVersion: a.SessionVersion, SecurityVersion: state.Version, DeviceHash: hashDeviceID(r.Header.Get("X-Device-ID")), Secret: sealed})
	if e != nil {
		writeError(w, 500, e)
		return
	}
	uri := "otpauth://totp/" + url.PathEscape("同帧:"+a.Email) + "?" + url.Values{"secret": {secret}, "issuer": {"同帧"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"challenge": token, "secret": secret, "uri": uri}})
}
func (s *Server) totpConfirm(w http.ResponseWriter, r *http.Request) {
	user, e := s.userFromRequest(r)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	if !s.enforceRateLimit(w, r, "security", user.ID, 10, 15*time.Minute) {
		return
	}
	var in securityInput
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	var c loginCeremony
	e = consumeChallenge(r.Context(), s.repo, in.Challenge, "totp-setup", &c)
	if e != nil || s.options.Vault == nil || c.UserID != user.ID || c.SessionVersion != user.SessionVersion || c.DeviceHash != hashDeviceID(r.Header.Get("X-Device-ID")) {
		writeError(w, 403, ErrInvalidActionToken)
		return
	}
	secret, e := s.options.Vault.Decrypt(c.Secret, "totp:"+user.ID)
	if e != nil {
		writeError(w, 403, ErrInvalidActionToken)
		return
	}
	step, ok := verifyTOTP(string(secret), in.Code, 0)
	if !ok {
		writeError(w, 400, errors.New("验证码不正确，请重新配置"))
		return
	}
	state, e := s.repo.ReadAccountSecurity(r.Context(), user.ID)
	if e != nil || state.Version != c.SecurityVersion || state.TOTPSecret != "" {
		writeError(w, 409, ErrAdminConflict)
		return
	}
	state.TOTPSecret = c.Secret
	state.LastStep = step
	codes := []string{}
	state.RecoveryHashes = []string{}
	for i := 0; i < 10; i++ {
		code := strings.ToUpper(mustRandomString(12))
		codes = append(codes, code)
		state.RecoveryHashes = append(state.RecoveryHashes, hashRefreshToken(code))
	}
	fresh, e := s.repo.WriteAccountSecurity(r.Context(), user.ID, user.SessionVersion, state, true)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	session, e := s.auth.issueSession(r.Context(), fresh, c.DeviceHash)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"session": session, "recovery_codes": codes}})
}
func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request) {
	user, e := s.userFromRequest(r)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	if !s.enforceRateLimit(w, r, "security", user.ID, 10, 15*time.Minute) {
		return
	}
	var in securityInput
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	_, state, e := s.reauthenticate(r.Context(), user, in)
	if e != nil || state.TOTPSecret == "" {
		writeError(w, 403, ErrInvalidCredentials)
		return
	}
	state.TOTPSecret = ""
	state.RecoveryHashes = nil
	state.LastStep = 0
	fresh, e := s.repo.WriteAccountSecurity(r.Context(), user.ID, user.SessionVersion, state, true)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	session, e := s.auth.issueSession(r.Context(), fresh, hashDeviceID(r.Header.Get("X-Device-ID")))
	if e != nil {
		writeError(w, 401, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
func (s *Server) emailLoginRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !s.enforceRateLimit(w, r, "email-login-ip", sourceIdentity(r), 15, time.Hour) || !s.enforceRateLimit(w, r, "email-login-account", email, 5, time.Hour) {
		return
	}
	c, _, e := readLoginConfig(r.Context(), s.repo)
	if e != nil || !c.EmailLogin {
		writeError(w, 403, errors.New("邮箱验证码登录未启用"))
		return
	}
	device, e := deviceFromRequest(r)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	challenge := mustRandomString(32)
	a, e := s.repo.UserByEmail(r.Context(), email)
	if e == nil && accountActive(a.User) && a.EmailVerified {
		n, e := rand.Int(rand.Reader, big.NewInt(100000000))
		if e != nil {
			writeError(w, 500, e)
			return
		}
		code := fmt.Sprintf("%08d", n.Int64())
		raw, _ := json.Marshal(loginCeremony{UserID: a.ID, SessionVersion: a.SessionVersion, DeviceHash: hashDeviceID(device.ID), CodeHash: hashRefreshToken(code)})
		if e = s.repo.SaveAuthChallenge(r.Context(), challenge, AuthChallenge{Purpose: "email-login", Data: raw, ExpiresAt: time.Now().Add(5 * time.Minute)}); e != nil {
			writeError(w, 503, errors.New("登录服务暂不可用"))
			return
		}
		_ = s.auth.mailer.Send(r.Context(), MailMessage{Type: "login_code", To: email, Token: code, ExpiresAt: time.Now().Add(5 * time.Minute)})
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"challenge": challenge}, Msg: "若该邮箱已验证且可用，登录验证码将发送至邮箱"})
}
func (s *Server) emailLoginFinish(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "email-login-finish", sourceIdentity(r), 15, 15*time.Minute) {
		return
	}
	var in securityInput
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	c, _, e := readLoginConfig(r.Context(), s.repo)
	if e != nil || !c.EmailLogin {
		writeError(w, 403, ErrForbidden)
		return
	}
	var ceremony loginCeremony
	e = consumeChallenge(r.Context(), s.repo, in.Challenge, "email-login", &ceremony)
	device, de := deviceFromRequest(r)
	if e != nil || de != nil || ceremony.DeviceHash != hashDeviceID(device.ID) || subtle.ConstantTimeCompare([]byte(ceremony.CodeHash), []byte(hashRefreshToken(in.Code))) != 1 {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	a, e := s.repo.UserByID(r.Context(), ceremony.UserID)
	if e != nil || !accountActive(a.User) || a.SessionVersion != ceremony.SessionVersion || !a.EmailVerified {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	if e = s.repo.BindDevice(r.Context(), a.ID, ceremony.DeviceHash, device); e != nil {
		writeError(w, 401, e)
		return
	}
	session, e := s.auth.beginAuthenticatedLogin(r.Context(), a.User, ceremony.DeviceHash)
	if writeLoginChallenge(w, e) {
		return
	}
	if e != nil {
		writeError(w, 401, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
func (s *Server) publicLoginOptions(w http.ResponseWriter, r *http.Request) {
	c, _, e := readLoginConfig(r.Context(), s.repo)
	if e != nil {
		writeError(w, 503, e)
		return
	}
	policy, _, err := s.sitePolicy(r.Context())
	if err != nil {
		writeError(w, 503, errors.New("注册设置暂不可用"))
		return
	}
	providers := []map[string]string{}
	for _, p := range c.Providers {
		if p.Enabled {
			providers = append(providers, map[string]string{"id": p.ID, "name": p.Name, "kind": p.Kind})
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"password_login": c.PasswordLogin, "email_login": c.EmailLogin, "passkeys": c.Passkeys, "providers": providers, "registration_enabled": policy.RegistrationEnabled, "registration_invite_required": policy.RegistrationRequiresInvite, "registration_email_code_required": policy.RegistrationRequiresEmailCode}})
}
func (s *Server) adminLoginConfig(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	c, version, e := readLoginConfig(r.Context(), s.repo)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	if r.Method == "PUT" {
		var in struct {
			Version int64       `json:"version"`
			Config  LoginConfig `json:"config"`
		}
		if e = decodeJSON(r, &in); e != nil {
			writeError(w, 400, e)
			return
		}
		next := in.Config
		if (next.Passkeys || len(next.Providers) > 0) && !validPublicBase(next.BaseURL) {
			writeError(w, 400, errors.New("请设置正式 HTTPS 地址，或本机 HTTP 测试地址"))
			return
		}

		alternative := next.EmailLogin || next.Passkeys
		for _, p := range next.Providers {
			alternative = alternative || p.Enabled
		}
		if !next.PasswordLogin && !alternative {
			writeError(w, 400, errors.New("请至少保留一种登录方式"))
			return
		}
		if len(next.Providers) > 12 || len(next.RPDisplayName) > 100 {
			writeError(w, 400, errors.New("配置超出允许范围"))
			return
		}
		seen := map[string]bool{}
		for i := range next.Providers {
			p := &next.Providers[i]
			if !serviceID.MatchString(p.ID) || seen[p.ID] || p.Name == "" || len(p.Name) > 60 || (p.Kind != "oidc" && p.Kind != "github") || (p.Kind == "oidc" && !validIssuer(p.Issuer)) || p.ClientID == "" {
				writeError(w, 400, errors.New("第三方登录配置无效"))
				return
			}
			seen[p.ID] = true
			p.SecretEnvelope = ""
			for _, old := range c.Providers {
				if old.ID == p.ID && old.ClientID == p.ClientID && old.Issuer == p.Issuer && old.Kind == p.Kind {
					p.SecretEnvelope = old.SecretEnvelope
				}
			}
			if p.ClientSecret != "" {
				if s.options.Vault == nil {
					writeError(w, 503, errors.New("密钥库未配置"))
					return
				}
				p.SecretEnvelope, e = s.options.Vault.Encrypt([]byte(p.ClientSecret), "oidc:"+p.ID)
				if e != nil {
					writeError(w, 500, e)
					return
				}
			}
			p.ClientSecret = ""
			if p.Enabled && p.SecretEnvelope == "" {
				writeError(w, 400, errors.New("请填写第三方登录密钥"))
				return
			}
		}
		raw, _ := json.Marshal(next)
		d, e := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "login", Version: in.Version, Data: raw}, actor.ID)
		if e != nil {
			writeAdminError(w, e)
			return
		}
		c = next
		version = d.Version
	}
	for i := range c.Providers {
		c.Providers[i].HasSecret = c.Providers[i].SecretEnvelope != ""
		c.Providers[i].SecretEnvelope = ""
		c.Providers[i].ClientSecret = ""
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": version, "config": c}})
}
