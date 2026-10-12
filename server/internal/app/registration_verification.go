package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type registrationVerification struct {
	Email      string `json:"email"`
	DeviceHash string `json:"device_hash"`
	CodeHash   string `json:"code_hash"`
}

func (s *Server) requestRegistrationCode(w http.ResponseWriter, r *http.Request) {
	policy, _, err := s.sitePolicy(r.Context())
	if err != nil {
		writeError(w, 503, errors.New("注册服务暂不可用"))
		return
	}
	if !policy.RegistrationEnabled {
		writeError(w, 403, errors.New("站点当前暂停注册"))
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if decodeJSON(r, &in) != nil {
		writeError(w, 400, errors.New("请填写有效邮箱"))
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		writeError(w, 400, errors.New("请填写有效邮箱"))
		return
	}
	device, err := deviceFromRequest(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if !s.enforceRateLimit(w, r, "registration-code-ip", sourceIdentity(r), 10, time.Hour) ||
		!s.enforceRateLimit(w, r, "registration-code-email-minute", email, 1, time.Minute) ||
		!s.enforceRateLimit(w, r, "registration-code-email-hour", email, 5, time.Hour) {
		return
	}
	if _, err := s.repo.UserByEmail(r.Context(), email); err == nil {
		writeError(w, 409, ErrEmailExists)
		return
	} else if !errors.Is(err, ErrInvalidCredentials) {
		writeError(w, 503, errors.New("注册服务暂不可用"))
		return
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		writeError(w, 503, errors.New("验证码生成失败"))
		return
	}
	code, challenge := fmt.Sprintf("%06d", n), mustRandomString(32)
	expiry := time.Now().Add(10 * time.Minute)
	data, _ := json.Marshal(registrationVerification{Email: email, DeviceHash: hashDeviceID(device.ID), CodeHash: hashRefreshToken(challenge + ":" + code)})
	if err = s.repo.SaveAuthChallenge(r.Context(), challenge, AuthChallenge{Purpose: "registration", Data: data, ExpiresAt: expiry}); err != nil {
		writeError(w, 503, errors.New("注册服务暂不可用"))
		return
	}
	if err = s.auth.mailer.Send(r.Context(), MailMessage{Type: "register_code", To: email, Token: code, ExpiresAt: expiry}); err != nil {
		_, _ = s.repo.ConsumeAuthChallenge(r.Context(), challenge, "registration")
		writeError(w, 503, errors.New("验证码暂时无法发送，邮件服务尚未配置或投递失败，请稍后再试"))
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"challenge": challenge, "expires_in": 600, "retry_after": 60}, Msg: "验证码已发送，请查看邮箱"})
}

func (s *Server) verifyRegistrationCode(ctx context.Context, email, device, challenge, code string) error {
	invalid := errors.New("邮箱验证码无效或已过期，请重新获取")
	if len(challenge) < 24 || len(challenge) > 128 || len(code) != 6 {
		return invalid
	}
	record, err := s.repo.ConsumeAuthChallenge(ctx, challenge, "registration")
	if err != nil {
		return invalid
	}
	var data registrationVerification
	if json.Unmarshal(record.Data, &data) != nil || data.Email != strings.ToLower(strings.TrimSpace(email)) || data.DeviceHash != hashDeviceID(device) {
		return invalid
	}
	if subtle.ConstantTimeCompare([]byte(data.CodeHash), []byte(hashRefreshToken(challenge+":"+code))) != 1 {
		return invalid
	}
	return nil
}

func hasRegisteredAccount(user User) bool {
	return user.Email != "" && user.GuestRoomCode == "" && !strings.HasSuffix(user.Email, "@invalid.local")
}
