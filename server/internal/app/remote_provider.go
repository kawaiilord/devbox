package app

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type RemoteProviderConfig struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Endpoint    string   `json:"endpoint"`
	Enabled     bool     `json:"enabled"`
	TLS         bool     `json:"tls"`
	InsecureTLS bool     `json:"insecure_tls"`
	CustomCA    string   `json:"custom_ca"`
	Timeout     int      `json:"timeout_seconds"`
	Types       []string `json:"types"`
	Secret      string   `json:"secret,omitempty"`
	Envelope    string   `json:"secret_envelope,omitempty"`
	HasSecret   bool     `json:"has_secret"`
}
type RemoteProviderList struct {
	Providers []RemoteProviderConfig `json:"providers"`
}
type remoteConnection struct {
	key  [32]byte
	conn *grpc.ClientConn
}
type RemoteProviders struct {
	repo        Repository
	vault       *CredentialVault
	mu          sync.Mutex
	connections map[string]remoteConnection
}

func NewRemoteProviders(repo Repository, vault *CredentialVault) *RemoteProviders {
	return &RemoteProviders{repo: repo, vault: vault, connections: map[string]remoteConnection{}}
}
func providerTarget(raw string) (string, error) {
	if strings.Contains(raw, "://") {
		u, e := url.Parse(raw)
		if e != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return "", errors.New("远程端点格式无效")
		}
		raw = u.Host
	}
	host, port, e := net.SplitHostPort(raw)
	if e != nil || host == "" || port == "" || strings.ContainsAny(host, "/\r\n@") {
		return "", errors.New("请输入远程主机:端口")
	}
	return raw, nil
}
func validateRemote(p RemoteProviderConfig) error {
	if !serviceID.MatchString(p.ID) || len([]rune(p.Name)) < 1 || len([]rune(p.Name)) > 64 || p.Timeout < 1 || p.Timeout > 120 || len(p.CustomCA) > 128<<10 {
		return errors.New("远程 Provider 配置无效")
	}
	if _, e := providerTarget(p.Endpoint); e != nil {
		return e
	}
	if strings.HasPrefix(p.Endpoint, "https://") && !p.TLS {
		return errors.New("HTTPS 端点必须启用 TLS")
	}
	if p.InsecureTLS && !p.TLS {
		return errors.New("跳过证书校验仅适用于 TLS")
	}
	if len(p.Types) == 0 || len(p.Types) > 3 {
		return errors.New("请选择服务类型")
	}
	seen := map[string]bool{}
	for _, kind := range p.Types {
		if (kind != "alist" && kind != "emby" && kind != "bilibili") || seen[kind] {
			return errors.New("此远程协议仅支持 Alist、Emby 和 Bilibili")
		}
		seen[kind] = true
	}
	if p.CustomCA != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(p.CustomCA)) {
			return errors.New("CA 证书无效")
		}
	}
	return nil
}
func (p *RemoteProviders) config(ctx context.Context, id, kind string) (RemoteProviderConfig, error) {
	var list RemoteProviderList
	if _, e := readServiceConfig(ctx, p.repo, "remote-providers", &list); e != nil {
		return RemoteProviderConfig{}, e
	}
	for _, c := range list.Providers {
		if c.ID == id {
			if !c.Enabled || kind != "" && !slices.Contains(c.Types, kind) {
				return c, ErrForbidden
			}
			return c, nil
		}
	}
	return RemoteProviderConfig{}, ErrNotFound
}
func (p *RemoteProviders) reset(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, c := range p.connections {
		if id == "" || id == key {
			c.conn.Close()
			delete(p.connections, key)
		}
	}
}
func (p *RemoteProviders) Close() { p.reset("") }
func (p *RemoteProviders) connect(c RemoteProviderConfig) (*grpc.ClientConn, error) {
	if e := validateRemote(c); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(c)
	key := sha256.Sum256(raw)
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, ok := p.connections[c.ID]; ok {
		if old.key == key {
			return old.conn, nil
		}
		old.conn.Close()
		delete(p.connections, c.ID)
	}
	target, e := providerTarget(c.Endpoint)
	if e != nil {
		return nil, e
	}
	var transport credentials.TransportCredentials = insecure.NewCredentials()
	if c.TLS {
		host, _, _ := net.SplitHostPort(target)
		conf := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: c.InsecureTLS}
		if c.CustomCA != "" {
			roots, e := x509.SystemCertPool()
			if e != nil {
				roots = x509.NewCertPool()
			}
			roots.AppendCertsFromPEM([]byte(c.CustomCA))
			conf.RootCAs = roots
		}
		transport = credentials.NewTLS(conf)
	}
	conn, e := grpc.NewClient("passthrough:///"+target, grpc.WithTransportCredentials(transport), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(8<<20)))
	if e != nil {
		return nil, e
	}
	p.connections[c.ID] = remoteConnection{key: key, conn: conn}
	return conn, nil
}
func (p *RemoteProviders) invoke(ctx context.Context, id, kind, method string, in, out proto.Message) error {
	c, e := p.config(ctx, id, kind)
	if e != nil {
		return e
	}
	conn, e := p.connect(c)
	if e != nil {
		return e
	}
	if p.vault == nil {
		return errors.New("远程 Provider 密钥库未配置")
	}
	secret, e := p.vault.Decrypt(c.Envelope, "remote-provider:"+id)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-provider-secret", string(secret))
	if e = conn.Invoke(ctx, method, in, out); e != nil {
		return fmt.Errorf("远程 Provider 调用失败（%s）", status.Code(e))
	}
	return nil
}
func (p *RemoteProviders) Health(ctx context.Context, id string) error {
	var out healthpb.HealthCheckResponse
	if e := p.invoke(ctx, id, "", "/grpc.health.v1.Health/Check", &healthpb.HealthCheckRequest{}, &out); e != nil {
		return e
	}
	if out.Status != healthpb.HealthCheckResponse_SERVING {
		return errors.New("远程 Provider 尚未就绪")
	}
	return nil
}
func (s *Server) adminRemoteProviders(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	list := RemoteProviderList{Providers: []RemoteProviderConfig{}}
	version, e := readServiceConfig(r.Context(), s.repo, "remote-providers", &list)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	if r.Method == "PUT" {
		if s.sources == nil || s.options.Vault == nil {
			writeError(w, 503, errors.New("请先配置媒体密钥库"))
			return
		}
		var in struct {
			Version   int64                  `json:"version"`
			Providers []RemoteProviderConfig `json:"providers"`
		}
		if e = decodeJSON(r, &in); e != nil {
			writeError(w, 400, e)
			return
		}
		if len(in.Providers) > 32 {
			writeError(w, 400, errors.New("远程服务数量超出上限"))
			return
		}
		seen := map[string]bool{}
		for i := range in.Providers {
			c := &in.Providers[i]
			if e = validateRemote(*c); e != nil {
				writeError(w, 400, e)
				return
			}
			if seen[c.ID] {
				writeError(w, 400, errors.New("Provider 标识重复"))
				return
			}
			seen[c.ID] = true
			c.Envelope = ""
			for _, old := range list.Providers {
				if old.ID == c.ID {
					c.Envelope = old.Envelope
				}
			}
			if c.Secret != "" {
				if len(c.Secret) > 2048 || strings.IndexFunc(c.Secret, func(r rune) bool { return r < 32 || r > 126 }) >= 0 {
					writeError(w, 400, errors.New("共享密钥必须为 ASCII 可打印字符"))
					return
				}
				c.Envelope, e = s.options.Vault.Encrypt([]byte(c.Secret), "remote-provider:"+c.ID)
				if e != nil {
					writeError(w, 500, e)
					return
				}
			}
			c.Secret = ""
			if c.Envelope == "" {
				writeError(w, 400, errors.New("共享密钥不能为空"))
				return
			}
		}
		next := RemoteProviderList{Providers: in.Providers}
		raw, _ := json.Marshal(next)
		d, e := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "remote-providers", Version: in.Version, Data: raw}, actor.ID)
		if e != nil {
			writeAdminError(w, e)
			return
		}
		s.sources.remote.reset("")
		list = next
		version = d.Version
	}
	for i := range list.Providers {
		list.Providers[i].HasSecret = list.Providers[i].Envelope != ""
		list.Providers[i].Envelope = ""
		list.Providers[i].Secret = ""
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": version, "providers": list.Providers}})
}
func (s *Server) remoteProviderProbe(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	if s.sources == nil {
		writeError(w, 503, errors.New("媒体服务未配置"))
		return
	}
	if !s.enforceRateLimit(w, r, "remote-probe", actor.ID, 12, time.Minute) {
		return
	}
	id := r.PathValue("id")
	s.sources.remote.reset(id)
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	e := s.sources.remote.Health(ctx, id)
	result := map[string]any{"connected": e == nil, "latency_ms": time.Since(start).Milliseconds()}
	if e != nil {
		result["error"] = e.Error()
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: result})
}
func (s *Server) remoteProviderCatalog(w http.ResponseWriter, r *http.Request) {
	if _, e := s.userFromRequest(r); e != nil {
		writeError(w, 401, e)
		return
	}
	var list RemoteProviderList
	if _, e := readServiceConfig(r.Context(), s.repo, "remote-providers", &list); e != nil {
		writeAdminError(w, e)
		return
	}
	items := []map[string]any{}
	for _, p := range list.Providers {
		if p.Enabled {
			items = append(items, map[string]any{"id": p.ID, "name": p.Name, "types": p.Types})
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"providers": items}})
}
