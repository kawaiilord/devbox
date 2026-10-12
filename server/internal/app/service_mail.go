package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type MailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
type SMTPConfig struct {
	Enabled        bool                    `json:"enabled"`
	Host           string                  `json:"host"`
	Port           int                     `json:"port"`
	Security       string                  `json:"security"`
	Username       string                  `json:"username"`
	Password       string                  `json:"password,omitempty"`
	SecretEnvelope string                  `json:"secret_envelope,omitempty"`
	HasPassword    bool                    `json:"has_password"`
	From           string                  `json:"from"`
	FromName       string                  `json:"from_name"`
	BaseURL        string                  `json:"base_url"`
	Templates      map[string]MailTemplate `json:"templates"`
}

func readServiceConfig(ctx context.Context, repo Repository, key string, out any) (int64, error) {
	d, e := repo.GetAdminDocument(ctx, key)
	if e != nil {
		return 0, e
	}
	if len(d.Data) > 0 {
		e = json.Unmarshal(d.Data, out)
	}
	return d.Version, e
}
func defaultSMTPConfig() SMTPConfig {
	return SMTPConfig{Port: 587, Security: "starttls", FromName: "同帧", Templates: map[string]MailTemplate{
		"verify_email":   {Subject: "验证你的同帧邮箱", Body: "请验证你的邮箱：{{.Link}}\n验证码：{{.Token}}\n有效期至 {{.ExpiresAt}}"},
		"reset_password": {Subject: "重设同帧密码", Body: "点击链接重设密码：{{.Link}}\n重设令牌：{{.Token}}\n有效期至 {{.ExpiresAt}}"},
		"register_code":  {Subject: "同帧注册验证码", Body: "你的注册验证码是 {{.Token}}，10 分钟内有效。请勿将验证码分享给他人。"},
		"login_code":     {Subject: "同帧登录验证码", Body: "你的登录验证码是 {{.Token}}，有效期至 {{.ExpiresAt}}。"},
		"test":           {Subject: "同帧邮件测试", Body: "邮件服务已成功连接。"},
	}}
}
func validPublicBase(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/") && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())))
}
func validateSMTP(c SMTPConfig) error {
	if !c.Enabled {
		return nil
	}
	if c.Host == "" || strings.ContainsAny(c.Host, "/\r\n@") || c.Port < 1 || c.Port > 65535 {
		return errors.New("SMTP 主机和端口无效")
	}
	if c.Security != "tls" && c.Security != "starttls" && c.Security != "local" {
		return errors.New("请选择 TLS 或 STARTTLS")
	}
	if c.Security == "local" && c.Host != "localhost" && !net.ParseIP(c.Host).IsLoopback() {
		return errors.New("明文 SMTP 仅可用于本机测试")
	}
	a, e := mail.ParseAddress(c.From)
	if e != nil || a.Address != c.From || strings.ContainsAny(c.FromName, "\r\n") {
		return errors.New("发件人地址无效")
	}
	if !validPublicBase(c.BaseURL) {
		return errors.New("站点地址必须为 HTTPS，或本机 HTTP")
	}
	for k, t := range c.Templates {
		if _, ok := defaultSMTPConfig().Templates[k]; !ok {
			return errors.New("不支持的邮件模板类型")
		}
		if len(t.Body) > 16000 || len(t.Subject) > 200 || strings.ContainsAny(t.Subject, "\r\n") {
			return errors.New("邮件模板长度或标题无效")
		}
		if _, e = renderMailTemplate(t, map[string]string{"Token": "123456", "Link": "https://localhost", "ExpiresAt": "tomorrow"}); e != nil {
			return e
		}
	}
	return nil
}
func renderMailTemplate(t MailTemplate, data map[string]string) ([]byte, error) {
	parsed, e := template.New("message").Option("missingkey=error").Parse(t.Body)
	if e != nil {
		return nil, errors.New("邮件模板格式无效")
	}
	var b bytes.Buffer
	if e = parsed.Execute(&b, data); e != nil {
		return nil, errors.New("邮件模板仅支持 Token、Link 和 ExpiresAt")
	}
	return b.Bytes(), nil
}

type RuntimeMailer struct {
	repo     Repository
	vault    *CredentialVault
	fallback Mailer
}

func NewRuntimeMailer(repo Repository, vault *CredentialVault, fallback Mailer) *RuntimeMailer {
	return &RuntimeMailer{repo: repo, vault: vault, fallback: fallback}
}
func (m *RuntimeMailer) Send(ctx context.Context, message MailMessage) error {
	c := defaultSMTPConfig()
	if _, e := readServiceConfig(ctx, m.repo, "smtp", &c); e != nil {
		return e
	}
	if !c.Enabled {
		if message.Type == "register_code" {
			if _, noop := m.fallback.(NoopMailer); noop || m.fallback == nil {
				return errors.New("邮件服务尚未配置")
			}
		}
		if m.fallback != nil {
			return m.fallback.Send(ctx, message)
		}
		return errors.New("邮件服务尚未配置")
	}
	if c.SecretEnvelope != "" {
		if m.vault == nil {
			return errors.New("邮件密钥库未配置")
		}
		b, e := m.vault.Decrypt(c.SecretEnvelope, "service:smtp")
		if e != nil {
			return e
		}
		c.Password = string(b)
	}
	return sendSMTP(ctx, c, message)
}
func sendSMTP(ctx context.Context, c SMTPConfig, message MailMessage) error {
	if e := validateSMTP(c); e != nil {
		return e
	}
	if !c.Enabled {
		return errors.New("SMTP 服务已停用")
	}
	to, e := mail.ParseAddress(message.To)
	if e != nil || strings.ContainsAny(message.To, "\r\n") {
		return errors.New("收件人地址无效")
	}
	t, ok := c.Templates[message.Type]
	if !ok {
		t, ok = defaultSMTPConfig().Templates[message.Type]
	}
	if !ok {
		return errors.New("邮件模板不存在")
	}
	link := strings.TrimRight(c.BaseURL, "/") + "/admin/account.html#" + url.Values{"action": {message.Type}, "token": {message.Token}}.Encode()
	body, e := renderMailTemplate(t, map[string]string{"Token": message.Token, "Link": link, "ExpiresAt": message.ExpiresAt.UTC().Format(time.RFC3339)})
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dial := net.Dialer{Timeout: 10 * time.Second}
	conn, e := dial.DialContext(ctx, "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if e != nil {
		return errors.New("无法连接 SMTP 服务")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if c.Security == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if e = secure.HandshakeContext(ctx); e != nil {
			return errors.New("SMTP TLS 验证失败")
		}
		conn = secure
	}
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return errors.New("SMTP 握手失败")
	}
	defer client.Close()
	if c.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP 不支持 STARTTLS")
		}
		if e = client.StartTLS(tlsConfig); e != nil {
			return errors.New("SMTP STARTTLS 验证失败")
		}
	}
	if c.Username != "" {
		if e = client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); e != nil {
			return errors.New("SMTP 登录失败")
		}
	}
	if e = client.Mail(c.From); e != nil {
		return errors.New("SMTP 拒绝发件人")
	}
	if e = client.Rcpt(to.Address); e != nil {
		return errors.New("SMTP 拒绝收件人")
	}
	writer, e := client.Data()
	if e != nil {
		return errors.New("SMTP 不允许发送消息")
	}
	from := (&mail.Address{Name: c.FromName, Address: c.From}).String()
	_, e = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", from, to.String(), mime.QEncoding.Encode("utf-8", t.Subject), time.Now().Format(time.RFC1123Z))
	if e != nil {
		return e
	}
	quoted := quotedprintable.NewWriter(writer)
	if _, e = quoted.Write(body); e != nil {
		return e
	}
	if e = quoted.Close(); e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return errors.New("SMTP 未接受邮件")
	}
	return client.Quit()
}
func (s *Server) adminSMTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	c := defaultSMTPConfig()
	version, e := readServiceConfig(r.Context(), s.repo, "smtp", &c)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	if r.Method == "PUT" {
		var in struct {
			Version       int64      `json:"version"`
			Config        SMTPConfig `json:"config"`
			ClearPassword bool       `json:"clear_password"`
		}
		if e = decodeJSON(r, &in); e != nil {
			writeError(w, 400, e)
			return
		}
		next := in.Config
		next.SecretEnvelope = c.SecretEnvelope
		if in.ClearPassword {
			next.SecretEnvelope = ""
		}
		if next.Password != "" {
			if s.options.Vault == nil {
				writeError(w, 503, errors.New("请先配置 SAMEFRAME_VAULT_KEY"))
				return
			}
			next.SecretEnvelope, e = s.options.Vault.Encrypt([]byte(next.Password), "service:smtp")
			if e != nil {
				writeError(w, 500, e)
				return
			}
		}
		next.Password = ""
		if e = validateSMTP(next); e != nil {
			writeError(w, 400, e)
			return
		}
		raw, _ := json.Marshal(next)
		d, e := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "smtp", Version: in.Version, Data: raw}, actor.ID)
		if e != nil {
			writeAdminError(w, e)
			return
		}
		version = d.Version
		c = next
	}
	c.HasPassword = c.SecretEnvelope != ""
	c.SecretEnvelope = ""
	c.Password = ""
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": version, "config": c}})
}
func (s *Server) adminSMTPTest(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	if !s.enforceRateLimit(w, r, "smtp-test", actor.ID, 3, time.Minute) {
		return
	}
	var in struct {
		To string `json:"to"`
	}
	if e := decodeJSON(r, &in); e != nil {
		writeError(w, 400, e)
		return
	}
	c := defaultSMTPConfig()
	if _, e := readServiceConfig(r.Context(), s.repo, "smtp", &c); e != nil || !c.Enabled {
		writeError(w, 400, errors.New("请先保存并启用 SMTP 设置"))
		return
	}
	e := NewRuntimeMailer(s.repo, s.options.Vault, nil).Send(r.Context(), MailMessage{Type: "test", To: in.To, ExpiresAt: time.Now()})
	if e != nil {
		writeError(w, 502, e)
		return
	}
	s.repo.AppendAudit(r.Context(), AuditEvent{ActorID: actor.ID, Action: "smtp.test", TargetType: "settings", TargetID: "smtp"})
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "测试邮件已交给 SMTP 服务"})
}

func SMTPConfigured(ctx context.Context, repo Repository, vault *CredentialVault) (bool, error) {
	c := defaultSMTPConfig()
	_, e := readServiceConfig(ctx, repo, "smtp", &c)
	if e != nil {
		return false, e
	}
	if !c.Enabled {
		return false, nil
	}
	if e = validateSMTP(c); e != nil {
		return false, e
	}
	if c.SecretEnvelope != "" {
		if vault == nil {
			return false, errors.New("mail vault unavailable")
		}
		if _, e = vault.Decrypt(c.SecretEnvelope, "service:smtp"); e != nil {
			return false, e
		}
	}
	return true, nil
}
