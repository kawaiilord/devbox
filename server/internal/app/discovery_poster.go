package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var discoveryDoubanImageHost = regexp.MustCompile(`^img[0-9]{1,2}\.doubanio\.com$`)

func normalizeDiscoveryPoster(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.Path, "..") || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	valid := (discoveryDoubanImageHost.MatchString(host) && strings.HasPrefix(u.Path, "/view/photo/")) || (host == "lain.bgm.tv" && strings.HasPrefix(u.Path, "/pic/cover/"))
	if !valid {
		return ""
	}
	u.Scheme = "https"
	u.Host = host
	return u.String()
}

func (s *Server) posterSignature(value, expires string) string {
	mac := hmac.New(sha256.New, s.auth.tokens.secret)
	mac.Write([]byte("sameframe:discovery-poster:v1\n" + value + "\n" + expires))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) discoveryPosterURL(raw string) string {
	if raw = normalizeDiscoveryPoster(raw); raw == "" {
		return ""
	}
	u := base64.RawURLEncoding.EncodeToString([]byte(raw))
	// Public poster capabilities have no account authority. Hour buckets keep
	// browser caching effective without exposing login tokens in image URLs.
	expires := strconv.FormatInt((time.Now().Unix()/3600+25)*3600, 10)
	q := url.Values{"u": {u}, "e": {expires}, "s": {s.posterSignature(u, expires)}}
	return "/api/v1/discovery/poster?" + q.Encode()
}

func (s *Server) discoveryPoster(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	encoded, expiry, signature := q.Get("u"), q.Get("e"), q.Get("s")
	expires, err := strconv.ParseInt(expiry, 10, 64)
	now := time.Now().Unix()
	if err != nil || expires <= now || expires > now+26*3600 || len(encoded) > 2048 || !hmac.Equal([]byte(signature), []byte(s.posterSignature(encoded, expiry))) {
		writeError(w, 403, ErrForbidden)
		return
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		writeError(w, 400, ErrInvalidControl)
		return
	}
	target := normalizeDiscoveryPoster(string(decoded))
	if target == "" || target != string(decoded) {
		writeError(w, 403, ErrForbidden)
		return
	}
	poster, err := s.options.Discovery.poster(r.Context(), target)
	if err != nil {
		writeError(w, 502, errors.New("海报暂时无法加载"))
		return
	}
	w.Header().Set("Content-Type", poster.kind)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Write(poster.data)
}

func (d *DiscoveryService) poster(ctx context.Context, target string) (discoveryPoster, error) {
	d.mu.Lock()
	cached, ok := d.posters[target]
	d.mu.Unlock()
	if ok && d.now().Sub(cached.at) < 6*time.Hour {
		return cached, nil
	}
	ch := d.flights.DoChan("poster:"+target, func() (any, error) {
		requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		select {
		case d.slots <- struct{}{}:
			defer func() { <-d.slots }()
		case <-requestCtx.Done():
			return nil, requestCtx.Err()
		}
		req, _ := http.NewRequestWithContext(requestCtx, "GET", target, nil)
		req.Header.Set("User-Agent", "SameFrame/1.1 (https://sameframe.top)")
		req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif")
		if discoveryDoubanImageHost.MatchString(req.URL.Hostname()) {
			req.Header.Set("Referer", "https://movie.douban.com/")
		} else {
			req.Header.Set("Referer", "https://bgm.tv/")
		}
		res, err := d.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, errors.New("poster upstream unavailable")
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, (3<<20)+1))
		if err != nil || len(data) > 3<<20 {
			return nil, errors.New("poster too large")
		}
		kind := http.DetectContentType(data)
		if kind != "image/jpeg" && kind != "image/png" && kind != "image/webp" && kind != "image/gif" {
			return nil, errors.New("invalid poster image")
		}
		item := discoveryPoster{data: data, kind: kind, at: d.now()}
		d.mu.Lock()
		defer d.mu.Unlock()
		if old, exists := d.posters[target]; exists {
			d.posterBytes -= len(old.data)
			delete(d.posters, target)
		}
		for d.posterBytes+len(data) > 16<<20 || len(d.posters) >= 64 {
			var oldest string
			var at time.Time
			for key, p := range d.posters {
				if oldest == "" || p.at.Before(at) {
					oldest, at = key, p.at
				}
			}
			if oldest == "" {
				break
			}
			d.posterBytes -= len(d.posters[oldest].data)
			delete(d.posters, oldest)
		}
		d.posters[target] = item
		d.posterBytes += len(data)
		return item, nil
	})
	select {
	case <-ctx.Done():
		return discoveryPoster{}, ctx.Err()
	case value := <-ch:
		if value.Err != nil {
			return discoveryPoster{}, value.Err
		}
		return value.Val.(discoveryPoster), nil
	}
}
