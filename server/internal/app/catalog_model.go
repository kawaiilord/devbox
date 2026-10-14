package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const catalogMaxBytes = 2 << 20
const catalogMaxSources = 64

type CatalogChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Group     string `json:"group"`
	UserAgent string `json:"user_agent,omitempty"`
}
type CatalogSource struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Kind     string           `json:"kind"`
	URL      string           `json:"url"`
	Channels []CatalogChannel `json:"channels,omitempty"`
}
type CatalogSubscription struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	URL           string          `json:"url"`
	Enabled       bool            `json:"enabled"`
	IntervalHours int             `json:"interval_hours"`
	Generation    int64           `json:"generation"`
	NextRun       int64           `json:"next_run"`
	LastRun       int64           `json:"last_run"`
	LastSuccess   int64           `json:"last_success"`
	Error         string          `json:"error"`
	Lease         string          `json:"lease,omitempty"`
	LeaseUntil    int64           `json:"lease_until,omitempty"`
	Sources       []CatalogSource `json:"sources"`
}
type CatalogHealth struct {
	CheckedAt      int64  `json:"checked_at"`
	LatencyMS      int64  `json:"latency_ms"`
	Failures       int    `json:"failures"`
	SuspendedUntil int64  `json:"suspended_until"`
	Error          string `json:"error"`
}
type CatalogChange struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Action   string `json:"action"`
	Channels int    `json:"channels,omitempty"`
}
type CatalogUpdate struct {
	At             int64           `json:"at"`
	SubscriptionID string          `json:"subscription_id"`
	Name           string          `json:"name"`
	Mode           string          `json:"mode"`
	Added          int             `json:"added"`
	Changed        int             `json:"changed"`
	Removed        int             `json:"removed"`
	Duplicates     int             `json:"duplicates"`
	Skipped        []string        `json:"skipped"`
	Changes        []CatalogChange `json:"changes"`
	Error          string          `json:"error,omitempty"`
}
type CatalogState struct {
	Revision      int64                    `json:"revision"`
	Subscriptions []CatalogSubscription    `json:"subscriptions"`
	Disabled      map[string]bool          `json:"disabled"`
	Health        map[string]CatalogHealth `json:"health"`
	History       []CatalogUpdate          `json:"history"`
}
type catalogPreview struct {
	Actor        string
	Expires      time.Time
	Revision     int64
	Subscription CatalogSubscription
	Update       CatalogUpdate
}
type catalogCacheEntry struct {
	Data    []byte
	Expires time.Time
}
type CatalogService struct {
	guoguo   *GuoguoService
	repo     Repository
	vault    *CredentialVault
	client   *http.Client
	now      func() time.Time
	mu       sync.Mutex
	previews map[string]catalogPreview
	cache    map[string]catalogCacheEntry
	flight   singleflight.Group
	slots    chan struct{}
}

func NewCatalogService(repo Repository, vault *CredentialVault) *CatalogService {
	c := newSourceHTTPClient(false)
	c.Timeout = 10 * time.Second
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("订阅重定向过多")
		}
		if _, e := catalogURL(r.URL.String()); e != nil {
			return e
		}
		if len(via) > 0 && (sourceAuthority(r.URL) != sourceAuthority(via[0].URL) || (via[0].URL.Scheme == "https" && r.URL.Scheme != "https")) {
			return errors.New("订阅跨域跳转被拒绝")
		}
		return nil
	}
	return &CatalogService{repo: repo, vault: vault, client: c, now: time.Now, previews: map[string]catalogPreview{}, cache: map[string]catalogCacheEntry{}, slots: make(chan struct{}, 8)}
}
func catalogHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:12])
}
func catalogURL(raw string) (*url.URL, error) {
	u, e := assetRemoteURL(strings.TrimSpace(raw))
	if e != nil || len(raw) > 4096 {
		return nil, errors.New("请输入不含用户名密码的 HTTP(S) 地址")
	}
	if u.Opaque != "" || strings.ContainsAny(raw, "\r\n\\") {
		return nil, errors.New("媒体源地址无效")
	}
	// DNS is checked on every connection by the transport, including redirects.
	if host := strings.ToLower(u.Hostname()); host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, errors.New("不允许访问本机或内网地址")
	}
	if ip := parseCatalogIP(u.Hostname()); ip {
		return nil, errors.New("不允许访问本机或内网地址")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.RawQuery = u.Query().Encode()
	return u, nil
}
func (c *CatalogService) read(ctx context.Context) (CatalogState, int64, error) {
	d, e := c.repo.GetAdminDocument(ctx, "media-catalog")
	st := CatalogState{Subscriptions: []CatalogSubscription{}, Disabled: map[string]bool{}, Health: map[string]CatalogHealth{}, History: []CatalogUpdate{}}
	if e != nil || len(d.Data) == 0 {
		return st, d.Version, e
	}
	if c.vault == nil {
		return st, d.Version, errors.New("媒体订阅密钥库未配置")
	}
	var box struct {
		Envelope string `json:"envelope"`
	}
	if e = json.Unmarshal(d.Data, &box); e != nil {
		return st, d.Version, e
	}
	raw, e := c.vault.Decrypt(box.Envelope, "service:media-catalog")
	if e != nil {
		return st, d.Version, e
	}
	e = json.Unmarshal(raw, &st)
	return st, d.Version, e
}
func (c *CatalogService) save(ctx context.Context, st CatalogState, version int64, actor, action string) error {
	if c.vault == nil {
		return errors.New("媒体订阅密钥库未配置")
	}
	raw, e := json.Marshal(st)
	if e != nil {
		return e
	}
	if len(raw) > 8<<20 {
		return errors.New("订阅总容量超过 8 MB，请减少频道或订阅数量")
	}
	box, e := c.vault.Encrypt(raw, "service:media-catalog")
	if e != nil {
		return e
	}
	data, _ := json.Marshal(map[string]string{"envelope": box})
	_, e = c.repo.SaveCatalogDocument(ctx, AdminDocument{Key: "media-catalog", Version: version, Data: data}, actor, action)
	return e
}
func (c *CatalogService) mutate(ctx context.Context, actor, action string, fn func(*CatalogState) error) error {
	for n := 0; n < 8; n++ {
		st, v, e := c.read(ctx)
		if e != nil {
			return e
		}
		if e = fn(&st); e != nil {
			return e
		}
		e = c.save(ctx, st, v, actor, action)
		if !errors.Is(e, ErrAdminConflict) {
			return e
		}
	}
	return ErrAdminConflict
}
func catalogSources(st CatalogState, enabledOnly bool) []CatalogSource {
	result := []CatalogSource{}
	seen := map[string]bool{}
	for _, sub := range st.Subscriptions {
		if enabledOnly && !sub.Enabled {
			continue
		}
		for _, src := range sub.Sources {
			if seen[src.ID] || (enabledOnly && st.Disabled[src.ID]) {
				continue
			}
			seen[src.ID] = true
			result = append(result, src)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
func (c *CatalogService) source(ctx context.Context, id string) (CatalogSource, error) {
	if id == guoguoSourceID && c.guoguo != nil {
		return CatalogSource{ID: guoguoSourceID, Name: "果果短剧", Kind: "guoguo"}, nil
	}
	st, _, e := c.read(ctx)
	if e != nil {
		return CatalogSource{}, e
	}
	for _, src := range catalogSources(st, true) {
		if src.ID == id {
			return src, nil
		}
	}
	return CatalogSource{}, errors.New("片源已停用或已从订阅移除")
}
func appendCatalogUpdate(st *CatalogState, update CatalogUpdate) {
	st.History = append([]CatalogUpdate{update}, st.History...)
	if len(st.History) > 40 {
		st.History = st.History[:40]
	}
}
func (c *CatalogService) health(ctx context.Context, id string, started time.Time, err error) {
	if ctx.Err() != nil {
		return
	}
	_ = c.mutate(ctx, "", "health", func(st *CatalogState) error {
		h := st.Health[id]
		h.CheckedAt = c.now().UnixMilli()
		h.LatencyMS = max(0, c.now().Sub(started).Milliseconds())
		if err == nil {
			h.Failures = 0
			h.SuspendedUntil = 0
			h.Error = ""
		} else {
			h.Failures++
			h.Error = "接口暂不可用或响应格式不兼容"
			if h.Failures >= 3 {
				h.SuspendedUntil = c.now().Add(5 * time.Minute).UnixMilli()
			}
		}
		st.Health[id] = h
		return nil
	})
}
