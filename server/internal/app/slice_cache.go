package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CacheConfig struct {
	Enabled    bool             `json:"enabled"`
	MaxBytes   int64            `json:"max_bytes"`
	TTL        int64            `json:"ttl_seconds"`
	PurgeEpoch int64            `json:"purge_epoch"`
	NodePurge  map[string]int64 `json:"node_purge,omitempty"`
}

func defaultCacheConfig() CacheConfig { return CacheConfig{MaxBytes: 128 << 20, TTL: 900} }

type cacheEntry struct {
	Size    int64       `json:"size"`
	Expires int64       `json:"expires"`
	Used    int64       `json:"used"`
	Status  int         `json:"status"`
	Header  http.Header `json:"header"`
}
type CacheStats struct {
	Node       string `json:"node"`
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
	Bytes      int64  `json:"bytes"`
	Entries    int    `json:"entries"`
	Hits       int64  `json:"hits"`
	Misses     int64  `json:"misses"`
	Evictions  int64  `json:"evictions"`
	PurgeEpoch int64  `json:"purge_epoch"`
	NodePurge  int64  `json:"node_purge"`
	UpdatedAt  int64  `json:"updated_at"`
}
type SliceCache struct {
	mu               sync.Mutex
	dir, node        string
	entries          map[string]cacheEntry
	stats            CacheStats
	epoch, nodeEpoch int64
	limit            int64
}

var cacheFileID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func NewSliceCache(dir, node string) (*SliceCache, error) {
	if dir == "" {
		return nil, nil
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	stat, err := os.Lstat(dir)
	if err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("cache directory must be a regular directory")
	}
	c := &SliceCache{dir: dir, node: node, entries: map[string]cacheEntry{}, stats: CacheStats{Node: node, Configured: true}}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() || f.Type()&os.ModeSymlink != 0 {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		if !strings.HasSuffix(f.Name(), ".json") || !cacheFileID.MatchString(id) {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(dir, f.Name()))
		var entry cacheEntry
		if e != nil || json.Unmarshal(raw, &entry) != nil {
			continue
		}
		info, e := os.Lstat(filepath.Join(dir, id+".bin"))
		if e != nil || !info.Mode().IsRegular() || info.Size() != entry.Size || entry.Size > 8<<20 || entry.Expires <= time.Now().UnixMilli() {
			os.Remove(filepath.Join(dir, f.Name()))
			os.Remove(filepath.Join(dir, id+".bin"))
			continue
		}
		c.entries[id] = entry
		c.stats.Bytes += entry.Size
	}
	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}
		id := strings.TrimSuffix(strings.TrimSuffix(f.Name(), ".json"), ".bin")
		_, valid := c.entries[id]
		if (cacheFileID.MatchString(id) && !valid) || strings.HasPrefix(f.Name(), "slice-") {
			_ = os.Remove(filepath.Join(dir, f.Name()))
		}
	}
	return c, nil
}
func (c *SliceCache) removeLocked(id string) {
	if entry, ok := c.entries[id]; ok {
		c.stats.Bytes -= entry.Size
		delete(c.entries, id)
		os.Remove(filepath.Join(c.dir, id+".bin"))
		os.Remove(filepath.Join(c.dir, id+".json"))
		c.stats.Evictions++
	}
}
func (c *SliceCache) configure(cfg CacheConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Enabled = cfg.Enabled
	c.limit = cfg.MaxBytes
	if c.epoch != cfg.PurgeEpoch || c.nodeEpoch != cfg.NodePurge[c.node] {
		for id := range c.entries {
			c.removeLocked(id)
		}
		c.epoch = cfg.PurgeEpoch
		c.nodeEpoch = cfg.NodePurge[c.node]
	}
	now := time.Now().UnixMilli()
	for id, entry := range c.entries {
		if entry.Expires <= now {
			c.removeLocked(id)
		}
	}
	c.evictLocked(cfg.MaxBytes)
}
func (c *SliceCache) evictLocked(limit int64) {
	if c.stats.Bytes <= limit {
		return
	}
	ids := make([]string, 0, len(c.entries))
	for id := range c.entries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return c.entries[ids[i]].Used < c.entries[ids[j]].Used })
	for _, id := range ids {
		if c.stats.Bytes <= limit {
			break
		}
		c.removeLocked(id)
	}
}
func (c *SliceCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats
	s.Entries = len(c.entries)
	s.PurgeEpoch = c.epoch
	s.NodePurge = c.nodeEpoch
	s.UpdatedAt = time.Now().UnixMilli()
	return s
}
func cacheEligible(ticket MediaTicket, method, rng, ifRange string) bool {
	if method != "GET" || ifRange != "" || strings.Contains(ticket.Kind, "hls") || strings.Contains(ticket.Kind, "dash") || ticket.Kind == "rtmp" {
		return false
	}
	if rng == "" {
		return ticket.Kind == "platform-asset"
	}
	parts := strings.Split(strings.TrimPrefix(rng, "bytes="), "-")
	if len(parts) != 2 || !strings.HasPrefix(rng, "bytes=") {
		return false
	}
	start, e := strconv.ParseInt(parts[0], 10, 64)
	end, ee := strconv.ParseInt(parts[1], 10, 64)
	return e == nil && ee == nil && start >= 0 && end >= start && end-start < 8<<20
}
func (c *SliceCache) Open(cfg CacheConfig, key string, fetch func() (*http.Response, error)) (*http.Response, error) {
	c.configure(cfg)
	if !cfg.Enabled {
		return fetch()
	}
	sum := sha256.Sum256([]byte(key))
	id := hex.EncodeToString(sum[:])
	c.mu.Lock()
	epoch, nodeEpoch := c.epoch, c.nodeEpoch
	if entry, ok := c.entries[id]; ok {
		data, e := os.ReadFile(filepath.Join(c.dir, id+".bin"))
		if e == nil && int64(len(data)) == entry.Size {
			c.stats.Hits++
			entry.Used = time.Now().UnixMilli()
			c.entries[id] = entry
			c.mu.Unlock()
			return &http.Response{StatusCode: entry.Status, Header: entry.Header.Clone(), ContentLength: entry.Size, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
		c.removeLocked(id)
	}
	c.stats.Misses++
	c.mu.Unlock()
	res, e := fetch()
	if e != nil {
		return nil, e
	}
	length := res.ContentLength
	if length < 0 {
		length, _ = strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	}
	if (res.StatusCode != 200 && res.StatusCode != 206) || length <= 0 || length > 8<<20 || length > cfg.MaxBytes || strings.Contains(strings.ToLower(res.Header.Get("Cache-Control")), "no-store") {
		return res, nil
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	res.Body.Close()
	if e != nil {
		return nil, e
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	if int64(len(data)) != length {
		return res, nil
	}
	headers := http.Header{}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := res.Header.Get(k); v != "" {
			headers.Set(k, v)
		}
	}
	entry := cacheEntry{Size: length, Expires: time.Now().Add(time.Duration(cfg.TTL) * time.Second).UnixMilli(), Used: time.Now().UnixMilli(), Status: res.StatusCode, Header: headers}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != epoch || c.nodeEpoch != nodeEpoch || !c.stats.Enabled || length > c.limit {
		return res, nil
	}
	if _, ok := c.entries[id]; ok {
		return res, nil
	}
	c.evictLocked(c.limit - length)
	file, e := os.CreateTemp(c.dir, "slice-")
	if e != nil {
		return res, nil
	}
	name := file.Name()
	defer os.Remove(name)
	_, e = file.Write(data)
	if ce := file.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return res, nil
	}
	if e = os.Rename(name, filepath.Join(c.dir, id+".bin")); e != nil {
		return res, nil
	}
	raw, _ := json.Marshal(entry)
	if e = os.WriteFile(filepath.Join(c.dir, id+".json"), raw, 0600); e != nil {
		os.Remove(filepath.Join(c.dir, id+".bin"))
		return res, nil
	}
	c.entries[id] = entry
	c.stats.Bytes += length
	return res, nil
}
func (s *Server) cacheHeartbeat(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		c := defaultCacheConfig()
		_, e := readServiceConfig(ctx, s.repo, "slice-cache", &c)
		if e == nil && s.sources != nil && s.sources.cache != nil {
			s.sources.cache.configure(c)
			if s.redis != nil {
				raw, _ := json.Marshal(s.sources.cache.Stats())
				s.redis.client.Set(ctx, s.redis.prefix+":cache:node:"+s.redis.nodeID, raw, 20*time.Second)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Server) adminCache(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	c := defaultCacheConfig()
	version, e := readServiceConfig(r.Context(), s.repo, "slice-cache", &c)
	if e != nil {
		writeAdminError(w, e)
		return
	}
	if r.Method == "PUT" {
		var in struct {
			Version  int64  `json:"version"`
			Enabled  bool   `json:"enabled"`
			MaxBytes int64  `json:"max_bytes"`
			TTL      int64  `json:"ttl_seconds"`
			Purge    bool   `json:"purge"`
			Node     string `json:"node"`
		}
		if e = decodeJSON(r, &in); e != nil {
			writeError(w, 400, e)
			return
		}
		if in.MaxBytes < 8<<20 || in.MaxBytes > 100<<30 || in.TTL < 10 || in.TTL > 86400 || len(in.Node) > 128 {
			writeError(w, 400, errors.New("缓存大小需为 8 MB–100 GB，有效期为 10–86400 秒"))
			return
		}
		c.Enabled = in.Enabled
		c.MaxBytes = in.MaxBytes
		c.TTL = in.TTL
		if in.Purge {
			if in.Node == "" {
				c.PurgeEpoch++
			} else {
				if c.NodePurge == nil {
					c.NodePurge = map[string]int64{}
				}
				if len(c.NodePurge) > 128 {
					c.NodePurge = map[string]int64{}
					c.PurgeEpoch++
				}
				c.NodePurge[in.Node]++
			}
		}
		raw, _ := json.Marshal(c)
		d, e := s.repo.PutAdminDocument(r.Context(), AdminDocument{Key: "slice-cache", Version: in.Version, Data: raw}, actor.ID)
		if e != nil {
			writeAdminError(w, e)
			return
		}
		version = d.Version
	}
	nodes := []CacheStats{}
	if s.sources != nil && s.sources.cache != nil {
		s.sources.cache.configure(c)
		nodes = append(nodes, s.sources.cache.Stats())
	}
	if s.redis != nil {
		var cursor uint64
		for {
			keys, next, err := s.redis.client.Scan(r.Context(), cursor, s.redis.prefix+":cache:node:*", 100).Result()
			if err != nil {
				writeError(w, 503, errors.New("缓存节点状态暂不可用"))
				return
			}
			for _, key := range keys {
				if key == s.redis.prefix+":cache:node:"+s.redis.nodeID {
					continue
				}
				raw, e := s.redis.client.Get(r.Context(), key).Bytes()
				var n CacheStats
				if e == nil && json.Unmarshal(raw, &n) == nil {
					nodes = append(nodes, n)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"version": version, "config": c, "nodes": nodes}})
}

func (m *MediaSourceManager) SetSliceCache(c *SliceCache) { m.cache = c }
