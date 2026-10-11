package app

import "net/http"

func (s *Server) registerSecurityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/smtp", s.adminSMTP)
	mux.HandleFunc("PUT /api/v1/admin/smtp", s.adminSMTP)
	mux.HandleFunc("POST /api/v1/admin/smtp/test", s.adminSMTPTest)
	mux.HandleFunc("GET /api/v1/admin/login-config", s.adminLoginConfig)
	mux.HandleFunc("PUT /api/v1/admin/login-config", s.adminLoginConfig)
	mux.HandleFunc("GET /api/v1/auth/options", s.publicLoginOptions)
	mux.HandleFunc("POST /api/v1/auth/browser/start", s.browserLoginStart)
	mux.HandleFunc("POST /api/v1/auth/browser/finish", s.browserLoginFinish)
	mux.HandleFunc("POST /api/v1/auth/browser/exchange", s.browserLoginExchange)
	mux.HandleFunc("POST /api/v1/auth/mfa", s.finishMFA)
	mux.HandleFunc("GET /api/v1/account/security", s.accountSecurity)
	mux.HandleFunc("POST /api/v1/account/totp/setup", s.totpSetup)
	mux.HandleFunc("POST /api/v1/account/totp/confirm", s.totpConfirm)
	mux.HandleFunc("POST /api/v1/account/totp/disable", s.totpDisable)
	mux.HandleFunc("POST /api/v1/auth/email/request", s.emailLoginRequest)
	mux.HandleFunc("POST /api/v1/auth/email/finish", s.emailLoginFinish)
	mux.HandleFunc("POST /api/v1/account/passkeys/begin", s.passkeyBegin)
	mux.HandleFunc("POST /api/v1/account/passkeys/finish", s.passkeyFinish)
	mux.HandleFunc("DELETE /api/v1/account/passkeys/{id}", s.passkeyDelete)
	mux.HandleFunc("POST /api/v1/auth/passkeys/begin", s.passkeyBegin)
	mux.HandleFunc("POST /api/v1/auth/passkeys/finish", s.passkeyFinish)
	mux.HandleFunc("POST /api/v1/auth/oauth/{id}/start", s.oauthStart)
	mux.HandleFunc("GET /api/v1/auth/oauth/{id}/callback", s.oauthCallback)
	mux.HandleFunc("POST /api/v1/auth/oauth/exchange", s.oauthExchange)
}
