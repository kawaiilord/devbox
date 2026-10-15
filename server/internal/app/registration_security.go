package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

const registrationWindow = 24 * time.Hour
const registrationBrowserCookie = "sf_registration_browser"

var ErrRegistrationSource = errors.New("暂时无法确认注册来源，请稍后重试")

// Browser and device signals are client claims. SourceIP is derived exclusively
// from the socket or the configured reverse proxy, never from a request body.
type RegistrationRecord struct {
	RegisteredAt    int64              `json:"registered_at"`
	Method          string             `json:"method"`
	CreatedBy       string             `json:"created_by,omitempty"`
	SourceIP        string             `json:"source_ip"`
	PeerIP          string             `json:"peer_ip"`
	IPSource        string             `json:"ip_source"`
	Network         string             `json:"network"`
	UserAgent       string             `json:"user_agent"`
	AcceptLanguage  string             `json:"accept_language"`
	ClientHintsUA   string             `json:"client_hints_ua"`
	ClientHintsOS   string             `json:"client_hints_os"`
	ClientMobile    string             `json:"client_mobile"`
	DeviceHash      string             `json:"device_hash"`
	DeviceName      string             `json:"device_name"`
	DevicePlatform  string             `json:"device_platform"`
	BrowserHash     string             `json:"browser_hash,omitempty"`
	EnvironmentHash string             `json:"environment_hash,omitempty"`
	ContextStatus   string             `json:"context_status"`
	Client          RegistrationClient `json:"client"`
}

type RegistrationClient struct {
	Timezone       string  `json:"timezone"`
	TimezoneOffset int     `json:"timezone_offset"`
	Language       string  `json:"language"`
	Platform       string  `json:"platform"`
	ScreenWidth    int     `json:"screen_width"`
	ScreenHeight   int     `json:"screen_height"`
	PixelRatio     float64 `json:"pixel_ratio"`
	WebDriver      bool    `json:"webdriver"`
}

type RegistrationMatch struct {
	UserID      string   `json:"user_id"`
	DisplayName string   `json:"display_name"`
	Reasons     []string `json:"reasons"`
}

type RegistrationDetails struct {
	Record       *RegistrationRecord `json:"record"`
	Related      []RegistrationMatch `json:"related"`
	RelatedTotal int                 `json:"related_total"`
}

type RegistrationRepository interface {
	ReadRegistration(context.Context, string) (RegistrationDetails, error)
}

type registrationLimitError struct {
	Bucket     string
	RetryAfter int64
}

func (e *registrationLimitError) Error() string {
	return "当前网络或设备注册次数已达上限，请稍后重试或联系管理员"
}

type registrationQuota struct {
	Start int64
	Count int
}

type registrationBucket struct {
	Key   string
	Kind  string
	Limit int
}

func registrationBuckets(record *RegistrationRecord, p SitePolicy) []registrationBucket {
	if record == nil || record.Method == "admin" {
		return nil
	}
	items := []registrationBucket{}
	for _, item := range []struct {
		kind, identity string
		limit          int
	}{{"network", record.Network, p.RegistrationIPLimit}, {"device", record.DeviceHash, p.RegistrationDeviceLimit}, {"browser", record.BrowserHash, p.RegistrationDeviceLimit}} {
		if item.identity != "" {
			items = append(items, registrationBucket{Key: item.kind + ":" + hashRefreshToken(item.identity), Kind: item.kind, Limit: item.limit})
		}
	}
	// All transactions acquire quota rows in the same order.
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items
}

func quotaNext(previous registrationQuota, now int64) registrationQuota {
	if previous.Start == 0 || previous.Start+registrationWindow.Milliseconds() <= now {
		return registrationQuota{Start: now, Count: 1}
	}
	previous.Count++
	return previous
}

func quotaExceeded(bucket registrationBucket, q registrationQuota, now int64) error {
	if bucket.Limit > 0 && q.Count > bucket.Limit {
		return &registrationLimitError{Bucket: bucket.Kind, RetryAfter: max(1, (q.Start+registrationWindow.Milliseconds()-now+999)/1000)}
	}
	return nil
}

func registrationIP(raw string) string {
	ip, err := netip.ParseAddr(raw)
	if err != nil || ip.Zone() != "" {
		return ""
	}
	return ip.Unmap().String()
}

func registrationNetwork(raw string) string {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	ip = ip.Unmap()
	if ip.Is4() {
		return ip.String()
	}
	return netip.PrefixFrom(ip, 64).Masked().String()
}

func registrationText(raw string, limit int) string {
	runes := []rune(strings.Join(strings.Fields(raw), " "))
	return string(runes[:min(len(runes), limit)])
}

func (s *Server) registrationDigest(raw string) string {
	mac := hmac.New(sha256.New, s.auth.tokens.secret)
	mac.Write([]byte("sameframe-registration-v1\x00" + raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) registrationBrowser(w http.ResponseWriter, r *http.Request) string {
	raw := ""
	if cookie, err := r.Cookie(registrationBrowserCookie); err == nil && len(cookie.Value) <= 180 {
		value, signature, ok := strings.Cut(cookie.Value, ".")
		if ok && len(value) >= 32 && hmac.Equal([]byte(signature), []byte(s.registrationDigest("cookie:"+value))) {
			raw = value
		}
	}
	if raw == "" {
		raw = mustRandomString(32)
		http.SetCookie(w, &http.Cookie{Name: registrationBrowserCookie, Value: raw + "." + s.registrationDigest("cookie:"+raw), Path: "/", HttpOnly: true,
			Secure: r.TLS != nil || strings.HasPrefix(s.options.PublicBaseURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 3600})
	}
	return s.registrationDigest("browser:" + raw)
}

func (s *Server) registrationFromRequest(w http.ResponseWriter, r *http.Request, device DeviceInfo, method string) *RegistrationRecord {
	provenance, _ := r.Context().Value(proxyProvenanceKey{}).(proxyProvenance)
	if provenance.Peer == "" {
		provenance = proxyProvenance{Peer: sourceIdentity(r), Source: "direct"}
	}
	record := &RegistrationRecord{Method: method, SourceIP: registrationIP(sourceIdentity(r)), PeerIP: registrationIP(provenance.Peer), IPSource: provenance.Source,
		UserAgent: registrationText(r.UserAgent(), 768), AcceptLanguage: registrationText(r.Header.Get("Accept-Language"), 256),
		ClientHintsUA: registrationText(r.Header.Get("Sec-CH-UA"), 512), ClientHintsOS: registrationText(r.Header.Get("Sec-CH-UA-Platform"), 80), ClientMobile: registrationText(r.Header.Get("Sec-CH-UA-Mobile"), 8),
		DeviceHash: hashDeviceID(device.ID), DeviceName: registrationText(device.Label, 64), DevicePlatform: registrationText(device.Platform, 32),
		BrowserHash: s.registrationBrowser(w, r), ContextStatus: "not_reported"}
	record.Network = registrationNetwork(record.SourceIP)
	if raw := r.Header.Get("X-Client-Context"); raw != "" {
		record.ContextStatus = "invalid"
		if len(raw) <= 2048 {
			body, err := base64.StdEncoding.DecodeString(raw)
			var client RegistrationClient
			if err == nil && json.Unmarshal(body, &client) == nil && client.ScreenWidth >= 0 && client.ScreenWidth <= 32768 && client.ScreenHeight >= 0 && client.ScreenHeight <= 32768 && client.PixelRatio >= 0 && client.PixelRatio <= 16 && client.TimezoneOffset >= -1440 && client.TimezoneOffset <= 1440 {
				client.Timezone = registrationText(client.Timezone, 80)
				client.Language = registrationText(client.Language, 64)
				client.Platform = registrationText(client.Platform, 80)
				record.Client, record.ContextStatus = client, "client_reported"
			}
		}
	}
	s.hashRegistrationEnvironment(record)
	return record
}

func (s *Server) hashRegistrationEnvironment(record *RegistrationRecord) {
	if record.UserAgent == "" {
		return
	}
	body, _ := json.Marshal([]any{record.UserAgent, record.AcceptLanguage, record.ClientHintsUA, record.ClientHintsOS, record.ClientMobile, record.DevicePlatform, record.Client})
	record.EnvironmentHash = s.registrationDigest("environment:" + string(body))
}

func validateRegistrationRecord(record *RegistrationRecord) error {
	if record != nil && (record.SourceIP == "" || record.IPSource == "unverified_proxy") {
		return ErrRegistrationSource
	}
	return nil
}

func registrationReasons(a, b RegistrationRecord) []string {
	reasons := []string{}
	for _, pair := range []struct{ name, a, b string }{{"ip", a.SourceIP, b.SourceIP}, {"network", a.Network, b.Network}, {"device", a.DeviceHash, b.DeviceHash}, {"browser", a.BrowserHash, b.BrowserHash}, {"environment", a.EnvironmentHash, b.EnvironmentHash}} {
		if pair.a != "" && pair.a == pair.b {
			reasons = append(reasons, pair.name)
		}
	}
	return reasons
}

func (s *Server) logRegistrationBlock(ctx context.Context, record *RegistrationRecord, err error) {
	var limit *registrationLimitError
	if record != nil && errors.As(err, &limit) {
		// A rejected signup has no authenticated actor. Administrative audit
		// events require an existing account, so use the service security log.
		s.options.Logger.WarnContext(ctx, "registration blocked", "bucket", limit.Bucket, "source_ip", record.SourceIP, "device_hash", record.DeviceHash, "browser_hash", record.BrowserHash)
	}
}

func (s *Server) writeRegistrationError(w http.ResponseWriter, r *http.Request, record *RegistrationRecord, err error) bool {
	if errors.Is(err, ErrDeviceBlocked) {
		writeError(w, http.StatusForbidden, errors.New("当前设备已限制注册，请联系管理员"))
		return true
	}
	var limit *registrationLimitError
	if errors.As(err, &limit) {
		s.logRegistrationBlock(r.Context(), record, err)
		w.Header().Set("Retry-After", strconv.FormatInt(limit.RetryAfter, 10))
		writeError(w, http.StatusTooManyRequests, err)
		return true
	}
	if errors.Is(err, ErrRegistrationSource) {
		writeError(w, http.StatusServiceUnavailable, err)
		return true
	}
	return false
}

func (s *Server) adminUserRegistration(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, "user.manage"); !ok {
		return
	}
	data, err := s.repo.ReadRegistration(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: data})
}
