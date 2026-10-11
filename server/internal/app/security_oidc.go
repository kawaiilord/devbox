package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var serviceID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

func validIssuer(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))
}

type oauthCeremony struct {
	Provider    string     `json:"provider"`
	Version     int64      `json:"version"`
	Verifier    string     `json:"verifier"`
	Nonce       string     `json:"nonce"`
	CookieHash  string     `json:"cookie_hash"`
	ResultToken string     `json:"result_token"`
	Device      DeviceInfo `json:"device"`
	LinkUser    string     `json:"link_user"`
	LinkVersion int64      `json:"link_version"`
}
type oauthResult struct {
	Login    loginCeremony `json:"login"`
	Error    string        `json:"error"`
	Linked   bool          `json:"linked"`
	Provider string        `json:"provider"`
	Version  int64         `json:"version"`
}

func identityNamespace(p IdentityProvider) string {
	if p.Kind == "github" {
		return "github:https://github.com"
	}
	return "oidc:" + p.Issuer
}
func (s *Server) oauthProvider(ctx context.Context, id string) (IdentityProvider, LoginConfig, int64, error) {
	c, v, e := readLoginConfig(ctx, s.repo)
	if e != nil {
		return IdentityProvider{}, c, v, e
	}
	for _, p := range c.Providers {
		if p.ID == id && p.Enabled {
			if s.options.Vault == nil {
				return p, c, v, ErrForbidden
			}
			b, e := s.options.Vault.Decrypt(p.SecretEnvelope, "oidc:"+p.ID)
			p.ClientSecret = string(b)
			return p, c, v, e
		}
	}
	return IdentityProvider{}, c, v, ErrNotFound
}
func oauthHTTPContext(ctx context.Context, p IdentityProvider) context.Context {
	u, _ := url.Parse(p.Issuer)
	allowLocal := u != nil && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())
	client := newSourceHTTPClient(allowLocal)
	client.Timeout = 12 * time.Second
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}
func oauthSettings(ctx context.Context, p IdentityProvider, c LoginConfig) (*oauth2.Config, *oidc.Provider, error) {
	conf := &oauth2.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, RedirectURL: strings.TrimRight(c.BaseURL, "/") + "/api/v1/auth/oauth/" + p.ID + "/callback"}
	if p.Kind == "github" {
		conf.Endpoint = oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token"}
		conf.Scopes = []string{"read:user", "user:email"}
		return conf, nil, nil
	}
	provider, e := oidc.NewProvider(ctx, p.Issuer)
	if e != nil {
		return nil, nil, errors.New("无法读取 OIDC 发现配置")
	}
	conf.Endpoint = provider.Endpoint()
	conf.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	return conf, provider, nil
}
func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "oauth-start", sourceIdentity(r), 20, 15*time.Minute) {
		return
	}
	p, c, v, e := s.oauthProvider(r.Context(), r.PathValue("id"))
	if e != nil {
		writeError(w, 403, errors.New("第三方登录不可用"))
		return
	}
	device, e := deviceFromRequest(r)
	if e != nil {
		writeError(w, 400, e)
		return
	}
	var in struct {
		securityInput
		Link bool `json:"link"`
	}
	if e = decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	ceremony := oauthCeremony{Provider: p.ID, Version: v, Verifier: oauth2.GenerateVerifier(), Nonce: mustRandomString(24), ResultToken: mustRandomString(32), Device: device}
	if in.Link {
		user, e := s.userFromRequest(r)
		if e != nil {
			writeError(w, 401, e)
			return
		}
		a, state, e := s.reauthenticate(r.Context(), user, in.securityInput)
		if e != nil {
			writeError(w, 403, ErrInvalidCredentials)
			return
		}
		if _, e = s.repo.WriteAccountSecurity(r.Context(), user.ID, user.SessionVersion, state, false); e != nil {
			writeAdminError(w, e)
			return
		}
		ceremony.LinkUser = a.ID
		ceremony.LinkVersion = a.SessionVersion
	}
	cookie := mustRandomString(32)
	ceremony.CookieHash = hashRefreshToken(cookie)
	ctx, cancel := context.WithTimeout(oauthHTTPContext(r.Context(), p), 15*time.Second)
	defer cancel()
	conf, _, e := oauthSettings(ctx, p, c)
	if e != nil {
		writeError(w, 502, e)
		return
	}
	state, e := saveChallenge(r.Context(), s.repo, "oauth-state", ceremony)
	if e != nil {
		writeError(w, 500, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "sf_oauth_" + p.ID, Value: cookie, Path: "/api/v1/auth/oauth/" + p.ID + "/callback", HttpOnly: true, Secure: strings.HasPrefix(c.BaseURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"url": conf.AuthCodeURL(state, oauth2.S256ChallengeOption(ceremony.Verifier), oidc.Nonce(ceremony.Nonce)), "poll_token": ceremony.ResultToken}})
}

type externalProfile struct {
	Subject, Email, Name string
	Verified             bool
}

func resolveExternalProfile(ctx context.Context, p IdentityProvider, conf *oauth2.Config, provider *oidc.Provider, token *oauth2.Token, nonce string) (externalProfile, error) {
	var out externalProfile
	if p.Kind == "oidc" {
		raw, _ := token.Extra("id_token").(string)
		id, e := provider.Verifier(&oidc.Config{ClientID: p.ClientID}).Verify(ctx, raw)
		if e != nil || id.Nonce != nonce {
			return out, ErrInvalidCredentials
		}
		var claims struct {
			Email           string `json:"email"`
			Name            string `json:"name"`
			Verified        bool   `json:"email_verified"`
			AuthorizedParty string `json:"azp"`
		}
		if e = id.Claims(&claims); e != nil {
			return out, e
		}
		if (len(id.Audience) > 1 && claims.AuthorizedParty != p.ClientID) || (claims.AuthorizedParty != "" && claims.AuthorizedParty != p.ClientID) {
			return out, ErrInvalidCredentials
		}
		return externalProfile{Subject: id.Subject, Email: claims.Email, Name: claims.Name, Verified: claims.Verified}, nil
	}
	client := conf.Client(ctx, token)
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if e := githubJSON(ctx, client, "https://api.github.com/user", &user); e != nil {
		return out, e
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if e := githubJSON(ctx, client, "https://api.github.com/user/emails", &emails); e != nil {
		return out, e
	}
	out.Subject = strconv.FormatInt(user.ID, 10)
	out.Name = user.Name
	if out.Name == "" {
		out.Name = user.Login
	}
	for _, email := range emails {
		if email.Verified && email.Primary {
			out.Email = email.Email
			out.Verified = true
			break
		}
	}
	if user.ID < 1 {
		return out, ErrInvalidCredentials
	}
	return out, nil
}
func githubJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ErrInvalidCredentials
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	var c oauthCeremony
	e := consumeChallenge(r.Context(), s.repo, r.URL.Query().Get("state"), "oauth-state", &c)
	cookie, ce := r.Cookie("sf_oauth_" + r.PathValue("id"))
	if e != nil || ce != nil || c.Provider != r.PathValue("id") || subtle.ConstantTimeCompare([]byte(c.CookieHash), []byte(hashRefreshToken(cookie.Value))) != 1 {
		writeError(w, 400, ErrInvalidActionToken)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "sf_oauth_" + c.Provider, Value: "", Path: "/api/v1/auth/oauth/" + c.Provider + "/callback", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteLaxMode})
	result := oauthResult{Provider: c.Provider, Version: c.Version, Error: "第三方登录失败，请重试"}
	err := func() error {
		p, config, v, e := s.oauthProvider(r.Context(), c.Provider)
		if e != nil || v != c.Version || r.URL.Query().Get("error") != "" {
			return ErrInvalidCredentials
		}
		ctx, cancel := context.WithTimeout(oauthHTTPContext(r.Context(), p), 25*time.Second)
		defer cancel()
		conf, provider, e := oauthSettings(ctx, p, config)
		if e != nil {
			return e
		}
		token, e := conf.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(c.Verifier))
		if e != nil {
			return e
		}
		profile, e := resolveExternalProfile(ctx, p, conf, provider, token, c.Nonce)
		if e != nil || profile.Subject == "" {
			return ErrInvalidCredentials
		}
		namespace := identityNamespace(p)
		if c.LinkUser != "" {
			a, e := s.repo.UserByID(ctx, c.LinkUser)
			if e != nil || !accountActive(a.User) || a.SessionVersion != c.LinkVersion {
				return ErrUnauthorized
			}
			if e = s.repo.LinkExternalIdentity(ctx, a.ID, namespace, profile.Subject); e != nil {
				result.Error = "该第三方身份已绑定，无法重复关联"
				return e
			}
			result.Linked = true
			result.Error = ""
			return nil
		}
		a, e := s.repo.ExternalIdentity(ctx, namespace, profile.Subject)
		if errors.Is(e, ErrNotFound) {
			if !profile.Verified {
				result.Error = "第三方平台未提供已验证邮箱，请先注册账号后在安全设置中关联"
				return ErrForbidden
			}
			email, err := validateCredentials(profile.Email, "External user", "external-generated-password")
			if err != nil {
				return err
			}
			if _, err = s.repo.UserByEmail(ctx, email); err == nil {
				result.Error = "此邮箱已有账号，请先用原方式登录，再在账号安全中关联第三方身份"
				return ErrForbidden
			} else if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrInvalidCredentials) {
				return err
			}
			policy, _, err := loadSitePolicy(ctx, s.repo)
			if err != nil || !policy.RegistrationEnabled {
				result.Error = "本站已关闭新账号注册"
				return ErrForbidden
			}
			name := strings.TrimSpace(profile.Name)
			if len([]rune(name)) < 2 {
				name = "新用户"
			}
			if len([]rune(name)) > 32 {
				name = string([]rune(name)[:32])
			}
			hash, err := hashPassword(mustRandomString(32))
			if err != nil {
				return err
			}
			a = AccountRecord{User: User{ID: mustRandomString(12), Email: email, DisplayName: name, SessionVersion: 1, EmailVerified: true, AccountStatus: "active"}, PasswordHash: hash, CreatedAt: time.Now()}
			review, err := s.repo.RegisterAccount(ctx, a, policy.RegistrationRequiresReview)
			if err != nil {
				return err
			}
			if err = s.repo.VerifyEmail(ctx, a.ID); err != nil {
				return err
			}
			if err = s.repo.LinkExternalIdentity(ctx, a.ID, namespace, profile.Subject); err != nil {
				return err
			}
			if review.ID != "" {
				result.Error = "注册申请已提交，请等待管理员审核后再次登录"
				return ErrForbidden
			}
			a, e = s.repo.UserByID(ctx, a.ID)
		}
		if e != nil || !accountActive(a.User) {
			result.Error = "账号尚未通过审核或已停用"
			return ErrForbidden
		}
		result.Login = loginCeremony{UserID: a.ID, SessionVersion: a.SessionVersion, DeviceHash: hashDeviceID(c.Device.ID)}
		result.Error = ""
		return nil
	}()
	_ = err
	raw, _ := json.Marshal(result)
	if e = s.repo.SaveAuthChallenge(r.Context(), c.ResultToken, AuthChallenge{Purpose: "oauth-result", Data: raw, ExpiresAt: time.Now().Add(2 * time.Minute)}); e != nil {
		writeError(w, 503, errors.New("登录结果暂不可用，请重试"))
		return
	}
	http.Redirect(w, r, "/admin/account.html#oauth_done=1", http.StatusSeeOther)
}
func (s *Server) oauthExchange(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRateLimit(w, r, "oauth-poll", sourceIdentity(r), 100, time.Minute) {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	var result oauthResult
	e := consumeChallenge(r.Context(), s.repo, in.Token, "oauth-result", &result)
	if e != nil {
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"pending": true}})
		return
	}
	if result.Error != "" {
		writeError(w, 403, errors.New(result.Error))
		return
	}
	if result.Linked {
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"linked": true}})
		return
	}
	_, _, v, e := s.oauthProvider(r.Context(), result.Provider)
	if e != nil || v != result.Version {
		writeError(w, 403, ErrForbidden)
		return
	}
	device, e := deviceFromRequest(r)
	if e != nil || hashDeviceID(device.ID) != result.Login.DeviceHash {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	a, e := s.repo.UserByID(r.Context(), result.Login.UserID)
	if e != nil || !accountActive(a.User) || a.SessionVersion != result.Login.SessionVersion {
		writeError(w, 401, ErrInvalidCredentials)
		return
	}
	if e = s.repo.BindDevice(r.Context(), a.ID, result.Login.DeviceHash, device); e != nil {
		writeError(w, 403, e)
		return
	}
	session, e := s.auth.beginAuthenticatedLogin(r.Context(), a.User, result.Login.DeviceHash)
	if writeLoginChallenge(w, e) {
		return
	}
	if e != nil {
		writeError(w, 401, e)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: session})
}
