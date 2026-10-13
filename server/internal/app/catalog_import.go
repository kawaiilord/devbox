package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func parseCatalogIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && disallowedSourceIP(ip)
}
func (c *CatalogService) fetch(ctx context.Context, raw string) ([]byte, error) {
	u, e := catalogURL(raw)
	if e != nil {
		return nil, e
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, errors.New("接口地址无效")
	}
	req.Header.Set("User-Agent", "SameFrame/1.0")
	req.Header.Set("Accept", "application/json, application/x-mpegURL, text/plain;q=0.9")
	response, e := c.client.Do(req)
	if e != nil {
		return nil, errors.New("接口连接失败或请求超时")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("接口返回 HTTP %d", response.StatusCode)
	}
	rawData, e := io.ReadAll(io.LimitReader(response.Body, catalogMaxBytes+1))
	if e != nil || len(rawData) > catalogMaxBytes {
		return nil, errors.New("接口响应读取失败或超过 2 MB")
	}
	return rawData, nil
}

var m3uAttribute = regexp.MustCompile(`([a-zA-Z0-9_-]+)="([^"]*)"`)

func parseCatalogM3U(data []byte, base string) ([]CatalogChannel, int, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if !strings.HasPrefix(text, "#EXTM3U") || !utf8.ValidString(text) {
		return nil, 0, errors.New("不是有效的 UTF-8 M3U 列表")
	}
	if strings.Contains(text, "#EXT-X-") {
		return nil, 0, errors.New("这是 HLS 视频分片清单，请导入频道 M3U 列表")
	}
	channels := []CatalogChannel{}
	seen := map[string]bool{}
	ids := map[string]bool{}
	skipped := 0
	name, group, tvgID := "", "", ""
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 4096), 32768)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#EXTINF:") {
			name, group, tvgID = "", "", ""
			if i := strings.LastIndex(line, ","); i >= 0 {
				name = strings.TrimSpace(line[i+1:])
			}
			for _, v := range m3uAttribute.FindAllStringSubmatch(line, -1) {
				switch v[1] {
				case "group-title":
					group = v[2]
				case "tvg-id":
					tvgID = v[2]
				}
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if name == "" || len([]rune(name)) > 150 {
			skipped++
			continue
		}
		raw := line
		if b, e := url.Parse(base); e == nil && b.Host != "" {
			if u, e := b.Parse(raw); e == nil {
				raw = u.String()
			}
		}
		u, e := catalogURL(raw)
		if e != nil {
			skipped++
			name = ""
			continue
		}
		raw = u.String()
		if seen[raw] {
			skipped++
			name = ""
			continue
		}
		seen[raw] = true
		identity := tvgID
		if identity == "" {
			identity = group + ":" + name
		}
		id := catalogHash(identity)
		if ids[id] {
			id = catalogHash(identity + ":" + raw)
		}
		ids[id] = true
		channels = append(channels, CatalogChannel{ID: id, Name: name, Group: catalogText(group, 100), URL: raw})
		name = ""
		if len(channels) > 2000 {
			return nil, skipped, errors.New("单个 M3U 列表最多 2000 个频道")
		}
	}
	if scanner.Err() != nil {
		return nil, skipped, errors.New("M3U 行过长或格式无效")
	}
	if len(channels) == 0 {
		return nil, skipped, errors.New("列表没有可用的 HTTP(S) 频道")
	}
	return channels, skipped, nil
}

func catalogText(value string, limit int) string {
	v := []rune(strings.TrimSpace(value))
	if len(v) > limit {
		v = v[:limit]
	}
	return string(v)
}
func anyString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return string(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}
func mapText(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := anyString(m[k]); s != "" {
			return s
		}
	}
	return ""
}
func canonicalCatalogAPI(raw string) (string, error) {
	u, e := catalogURL(raw)
	if e != nil {
		return "", e
	}
	q := u.Query()
	for _, k := range []string{"ac", "wd", "ids", "pg", "page", "t", "h"} {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *CatalogService) parseImport(ctx context.Context, data []byte, base, name string) ([]CatalogSource, []string, int, error) {
	if len(data) > catalogMaxBytes {
		return nil, nil, 0, errors.New("导入内容超过 2 MB")
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	skipped := []string{}
	duplicate := 0
	if bytes.HasPrefix(data, []byte("#EXTM3U")) {
		channels, n, e := parseCatalogM3U(data, base)
		if e != nil {
			return nil, nil, 0, e
		}
		idbase := base
		if idbase == "" {
			idbase = "upload:" + name
		}
		if n > 0 {
			skipped = append(skipped, fmt.Sprintf("跳过 %d 个重复或不兼容频道", n))
		}
		return []CatalogSource{{ID: catalogHash("m3u:" + idbase), Name: name, Kind: "m3u", URL: base, Channels: channels}}, skipped, n, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if decoder.Decode(&root) != nil {
		return nil, nil, 0, errors.New("需要标准 JSON 源列表、TVBOX JSON 或 M3U 文件")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, nil, 0, errors.New("JSON 内容不完整或含有额外数据")
	}
	entries := []map[string]any{}
	tvbox := false
	add := func(v any) {
		if list, ok := v.([]any); ok {
			for _, row := range list {
				if m, ok := row.(map[string]any); ok {
					entries = append(entries, m)
				} else {
					skipped = append(skipped, "跳过格式不正确的条目")
				}
			}
		}
	}
	switch v := root.(type) {
	case []any:
		add(v)
	case map[string]any:
		if _, ok := v["sites"]; ok {
			tvbox = true
			add(v["sites"])
		}
		add(v["sources"])
		if apis, ok := v["api_site"].(map[string]any); ok {
			keys := []string{}
			for k := range apis {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if m, ok := apis[k].(map[string]any); ok {
					if mapText(m, "name") == "" {
						m["name"] = k
					}
					entries = append(entries, m)
				}
			}
		}
		if lives, ok := v["lives"].([]any); ok {
			for _, live := range lives {
				if m, ok := live.(map[string]any); ok {
					if raw := mapText(m, "url"); raw != "" {
						entries = append(entries, map[string]any{"name": mapText(m, "name"), "url": raw, "type": "m3u"})
					} else {
						skipped = append(skipped, "跳过内嵌或专用直播规则；请提供标准 M3U 订阅")
					}
				}
			}
		}
		if len(entries) == 0 && mapText(v, "api", "url") != "" {
			entries = append(entries, v)
		}
	}
	if len(entries) > 256 {
		return nil, nil, 0, errors.New("每次最多导入 256 个条目")
	}
	result := []CatalogSource{}
	seen := map[string]bool{}
	m3uCount := 0
	for _, m := range entries {
		label := catalogText(mapText(m, "name", "key"), 64)
		if label == "" {
			label = "未命名片源"
		}
		raw := mapText(m, "api", "url")
		kind := strings.ToLower(mapText(m, "type", "kind"))
		if kind == "" {
			kind = "cms"
		}
		if kind == "1" {
			kind = "cms"
		}
		if kind == "0" && strings.Contains(raw, "/provide/vod") {
			kind = "cms"
		}
		if kind != "cms" && kind != "m3u" && kind != "m3u8" {
			skipped = append(skipped, label+"：脚本、爬虫或非标准接口不支持")
			continue
		}
		if tvbox && (mapText(m, "ext", "jar") != "") && kind != "m3u" {
			skipped = append(skipped, label+"：需要额外解析规则，已跳过")
			continue
		}
		var src CatalogSource
		src.Name = label
		if kind == "cms" {
			u, e := canonicalCatalogAPI(raw)
			if e != nil {
				skipped = append(skipped, label+"：地址无效")
				continue
			}
			src.Kind = "cms"
			src.URL = u
			src.ID = catalogHash("cms:" + u)
		} else {
			u, e := catalogURL(raw)
			if e != nil {
				skipped = append(skipped, label+"：M3U 地址无效")
				continue
			}
			src.Kind = "m3u"
			src.URL = u.String()
			src.ID = catalogHash("m3u:" + src.URL)
		}
		if seen[src.ID] {
			duplicate++
			continue
		}
		seen[src.ID] = true
		if src.Kind == "m3u" {
			m3uCount++
			if m3uCount > 8 {
				return nil, nil, 0, errors.New("每次最多更新 8 个 M3U 列表")
			}
			body, e := c.fetch(ctx, src.URL)
			if e != nil {
				return nil, nil, 0, fmt.Errorf("%s：M3U 获取失败，保留旧结果", label)
			}
			channels, n, e := parseCatalogM3U(body, src.URL)
			if e != nil {
				return nil, nil, 0, fmt.Errorf("%s：%s", label, e)
			}
			src.Channels = channels
			if n > 0 {
				skipped = append(skipped, fmt.Sprintf("%s：跳过 %d 个重复或无效频道", label, n))
			}
		}
		result = append(result, src)
	}
	if len(result) == 0 {
		return nil, skipped, duplicate, errors.New("没有兼容的标准接口或 M3U 频道；现有内容保持不变")
	}
	if len(result) > catalogMaxSources {
		return nil, nil, 0, errors.New("最多支持 64 个独立片源")
	}
	return result, skipped, duplicate, nil
}

func catalogDiff(old, next []CatalogSource) CatalogUpdate {
	u := CatalogUpdate{Changes: []CatalogChange{}, Skipped: []string{}}
	previous := map[string]CatalogSource{}
	for _, src := range old {
		previous[src.ID] = src
	}
	for _, src := range next {
		action := "unchanged"
		p, exists := previous[src.ID]
		if !exists {
			action = "added"
			u.Added++
		} else {
			a, _ := json.Marshal(p)
			b, _ := json.Marshal(src)
			if !bytes.Equal(a, b) {
				action = "changed"
				u.Changed++
			}
		}
		delete(previous, src.ID)
		u.Changes = append(u.Changes, CatalogChange{Name: src.Name, Kind: src.Kind, Action: action, Channels: len(src.Channels)})
	}
	for _, src := range previous {
		u.Removed++
		u.Changes = append(u.Changes, CatalogChange{Name: src.Name, Kind: src.Kind, Action: "removed", Channels: len(src.Channels)})
	}
	sort.SliceStable(u.Changes, func(i, j int) bool { return u.Changes[i].Name < u.Changes[j].Name })
	return u
}
