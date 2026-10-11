package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

func credentialID(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }

type passkeyUser struct {
	account User
	state   AccountSecurity
}

func (u passkeyUser) WebAuthnID() []byte          { return []byte(u.account.ID) }
func (u passkeyUser) WebAuthnName() string        { return u.account.Email }
func (u passkeyUser) WebAuthnDisplayName() string { return u.account.DisplayName }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	keys := []webauthn.Credential{}
	for _, k := range u.state.Passkeys {
		keys = append(keys, k.Credential)
	}
	return keys
}

type passkeyCeremony struct {
	Login   loginCeremony        `json:"login"`
	Session webauthn.SessionData `json:"session"`
	Name    string               `json:"name"`
	BaseURL string               `json:"base_url"`
}

func (s *Server) passkeyConfig(r *http.Request) (*webauthn.WebAuthn, string, error) {
	c, _, e := readLoginConfig(r.Context(), s.repo)
	if e != nil {
		return nil, "", e
	}
	if !c.Passkeys || !validPublicBase(c.BaseURL) {
		return nil, "", errors.New("Passkey 尚未启用，请先配置站点域名")
	}
	u, _ := url.Parse(c.BaseURL)
	origin := u.Scheme + "://" + u.Host
	auth, e := webauthn.New(&webauthn.Config{RPDisplayName: c.RPDisplayName, RPID: u.Hostname(), RPOrigins: []string{origin}, AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired, ResidentKey: protocol.ResidentKeyRequirementRequired}})
	return auth, origin, e
}
func (s *Server) passkeyBegin(w http.ResponseWriter, r *http.Request) {
	register := strings.Contains(r.URL.Path, "/account/")
	if !s.enforceRateLimit(w, r, "passkey", sourceIdentity(r), 20, 15*time.Minute) {
		return
	}
	wa, origin, e := s.passkeyConfig(r)
	if e != nil {
		writeError(w, 503, e)
		return
	}
	var in struct {
		securityInput
		Email string `json:"email"`
	}
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	device, e := deviceFromRequest(r)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	var a AccountRecord
	var state AccountSecurity
	if register {
		user, err := s.userFromRequest(r)
		if err != nil {
			writeError(w, 401, err)
			return
		}
		a, state, e = s.reauthenticate(r.Context(), user, in.securityInput)
	} else {
		a, e = s.repo.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(in.Email)))
		if e == nil {
			state, e = s.repo.ReadAccountSecurity(r.Context(), a.ID)
		}
	}
	if e != nil || !accountActive(a.User) {
		writeError(w, 403, ErrInvalidCredentials)
		return
	}
	u := passkeyUser{account: a.User, state: state}
	var opts any
	var ceremony *webauthn.SessionData
	purpose := "passkey-login"
	if register {
		if len(state.Passkeys) >= 10 || len(in.Name) > 80 {
			writeError(w, 400, errors.New("最多可添加 10 个 Passkey，名称最多 80 字节"))
			return
		}
		excludes := []protocol.CredentialDescriptor{}
		for _, k := range state.Passkeys {
			excludes = append(excludes, k.Credential.Descriptor())
		}
		opts, ceremony, e = wa.BeginRegistration(u, webauthn.WithExclusions(excludes))
		purpose = "passkey-register"
		if e == nil {
			_, e = s.repo.WriteAccountSecurity(r.Context(), a.ID, a.SessionVersion, state, false)
			state.Version++
		}
	} else {
		opts, ceremony, e = wa.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	}
	if e != nil {
		writeError(w, 400, errors.New("无法开始 Passkey 验证，请确认账号已添加 Passkey"))
		return
	}
	token, e := saveChallenge(r.Context(), s.repo, purpose, passkeyCeremony{Login: loginCeremony{UserID: a.ID, SessionVersion: a.SessionVersion, DeviceHash: hashDeviceID(device.ID), SecurityVersion: state.Version}, Session: *ceremony, Name: in.Name, BaseURL: origin})
	if e != nil {
		writeError(w, 500, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"challenge": token, "options": opts}})
}
func (s *Server) passkeyFinish(w http.ResponseWriter, r *http.Request) {
	register := strings.Contains(r.URL.Path, "/account/")
	if !s.enforceRateLimit(w, r, "passkey", sourceIdentity(r), 20, 15*time.Minute) {
		return
	}
	wa, origin, e := s.passkeyConfig(r)
	if e != nil {
		writeError(w, 503, e)
		return
	}
	var in struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	purpose := "passkey-login"
	if register {
		purpose = "passkey-register"
	}
	var c passkeyCeremony
	e = consumeChallenge(r.Context(), s.repo, in.Challenge, purpose, &c)
	device, de := deviceFromRequest(r)
	if e != nil || de != nil || c.BaseURL != origin || c.Login.DeviceHash != hashDeviceID(device.ID) {
		writeError(w, 401, ErrInvalidActionToken)
		return
	}
	a, e := s.repo.UserByID(r.Context(), c.Login.UserID)
	if e != nil || !accountActive(a.User) || a.SessionVersion != c.Login.SessionVersion {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	if register {
		user, e := s.userFromRequest(r)
		if e != nil || user.ID != a.ID {
			writeError(w, 401, ErrUnauthorized)
			return
		}
	}
	state, e := s.repo.ReadAccountSecurity(r.Context(), a.ID)
	if e != nil || state.Version != c.Login.SecurityVersion {
		writeError(w, 409, ErrAdminConflict)
		return
	}
	u := passkeyUser{account: a.User, state: state}
	var credential *webauthn.Credential
	if register {
		parsed, err := protocol.ParseCredentialCreationResponseBytes(in.Credential)
		if err == nil {
			credential, e = wa.CreateCredential(u, c.Session, parsed)
		} else {
			e = err
		}
	} else {
		parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
		if err == nil {
			credential, e = wa.ValidateLogin(u, c.Session, parsed)
		} else {
			e = err
		}
	}
	if e != nil || credential == nil || credential.Authenticator.CloneWarning {
		writeError(w, 401, errors.New("Passkey 验证失败，请检查域名、设备或重新开始"))
		return
	}
	if register {
		for _, k := range state.Passkeys {
			if credentialID(k.Credential.ID) == credentialID(credential.ID) {
				writeError(w, 409, ErrAdminConflict)
				return
			}
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = "Passkey"
		}
		state.Passkeys = append(state.Passkeys, NamedCredential{Name: name, CreatedAt: time.Now().UnixMilli(), Credential: *credential})
	} else {
		for i, k := range state.Passkeys {
			if credentialID(k.Credential.ID) == credentialID(credential.ID) {
				state.Passkeys[i].Credential = *credential
			}
		}
	}
	fresh, e := s.repo.WriteAccountSecurity(r.Context(), a.ID, a.SessionVersion, state, register)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	if e = s.repo.BindDevice(r.Context(), a.ID, c.Login.DeviceHash, device); e != nil {
		writeError(w, 403, e)
		return
	}
	session, e := s.auth.issueSession(r.Context(), fresh, c.Login.DeviceHash)
	if e != nil {
		writeError(w, 401, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
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
	if e != nil {
		writeError(w, 403, ErrInvalidCredentials)
		return
	}
	found := false
	for i, k := range state.Passkeys {
		if credentialID(k.Credential.ID) == r.PathValue("id") {
			state.Passkeys = append(state.Passkeys[:i:i], state.Passkeys[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		writeError(w, 404, ErrNotFound)
		return
	}
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
