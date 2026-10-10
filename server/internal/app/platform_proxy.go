package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type platformAsset struct {
	URL      string `json:"url"`
	Manifest bool   `json:"manifest"`
}

var hlsURIAttribute = regexp.MustCompile(`URI="([^"]+)"`)

func (s *Server) mediaTicketURL(raw string, ticket MediaTicket) string {
	base := s.mediaPlaybackURL(raw)
	switch ticket.Kind {
	case "platform-hls":
		return base + "/stream.m3u8"
	case "platform-dash":
		return base + "/stream.mpd"
	case "platform-file":
		if ticket.Container == "flv" {
			return base + "/stream.flv"
		}
	}
	return base
}

func (s *Server) platformAssetURL(raw, target string, manifest bool) (string, error) {
	if _, err := mediaRemoteURL(target); err != nil {
		return "", err
	}
	data, _ := json.Marshal(platformAsset{URL: target, Manifest: manifest})
	encrypted, err := s.sources.vault.Encrypt(data, "media-asset:"+raw)
	if err != nil {
		return "", err
	}
	suffix := "/asset"
	if manifest {
		suffix += ".m3u8"
	}
	return s.mediaPlaybackURL(raw) + suffix + "?asset=" + url.QueryEscape(encrypted), nil
}

func (s *Server) decodePlatformAsset(raw, encrypted string) (platformAsset, error) {
	if len(encrypted) > 24000 {
		return platformAsset{}, ErrUnauthorized
	}
	data, err := s.sources.vault.Decrypt(encrypted, "media-asset:"+raw)
	if err != nil {
		return platformAsset{}, ErrUnauthorized
	}
	var asset platformAsset
	if json.Unmarshal(data, &asset) != nil {
		return asset, ErrUnauthorized
	}
	if _, err = mediaRemoteURL(asset.URL); err != nil {
		return asset, ErrUnauthorized
	}
	return asset, nil
}

func (s *Server) rewriteHLS(raw, baseURL string, data []byte) ([]byte, error) {
	if len(data) > 2<<20 || !utf8.Valid(data) || !strings.HasPrefix(strings.TrimSpace(string(data)), "#EXTM3U") {
		return nil, errors.New("平台播放清单无效")
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	rewrite := func(value string, manifest bool) (string, error) {
		target, err := base.Parse(value)
		if err != nil {
			return "", errors.New("播放清单地址无效")
		}
		return s.platformAssetURL(raw, target.String(), manifest || strings.Contains(strings.ToLower(target.Path), ".m3u8"))
	}
	lines := strings.Split(string(data), "\n")
	nextManifest := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			lines[i], err = rewrite(trimmed, nextManifest)
			if err != nil {
				return nil, err
			}
			nextManifest = false
			continue
		}
		attributeManifest := strings.HasPrefix(trimmed, "#EXT-X-MEDIA:") || strings.HasPrefix(trimmed, "#EXT-X-I-FRAME-STREAM-INF:") || strings.HasPrefix(trimmed, "#EXT-X-RENDITION-REPORT:")
		var replaceErr error
		lines[i] = hlsURIAttribute.ReplaceAllStringFunc(line, func(attribute string) string {
			target := hlsURIAttribute.FindStringSubmatch(attribute)[1]
			rewritten, e := rewrite(target, attributeManifest)
			if e != nil {
				replaceErr = e
				return attribute
			}
			return `URI="` + rewritten + `"`
		})
		if replaceErr != nil {
			return nil, replaceErr
		}
		if strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF:") {
			nextManifest = true
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

type mp4Index struct{ initEnd, indexStart, indexEnd int64 }

func readMP4Index(data []byte) (mp4Index, error) {
	var result mp4Index
	foundInit := false
	for offset := 0; offset+8 <= len(data); {
		size := int64(binary.BigEndian.Uint32(data[offset : offset+4]))
		kind := string(data[offset+4 : offset+8])
		header := 8
		if size == 1 {
			if offset+16 > len(data) {
				break
			}
			size = int64(binary.BigEndian.Uint64(data[offset+8 : offset+16]))
			header = 16
		}
		if size < int64(header) || size > int64(len(data)-offset) {
			break
		}
		if kind == "moov" {
			result.initEnd = int64(offset) + size - 1
			foundInit = true
		}
		if kind == "sidx" && foundInit {
			result.indexStart = int64(offset)
			result.indexEnd = int64(offset) + size - 1
			return result, nil
		}
		offset += int(size)
	}
	return result, errors.New("该分离音视频码流没有可识别的分段索引，请选择其他画质")
}

func (m *MediaSourceManager) probeMP4Index(ctx context.Context, ticket MediaTicket, target string) (mp4Index, error) {
	ticket.StreamURL = target
	ticket.Kind = "platform-file"
	response, err := m.Open(ctx, ticket, http.MethodGet, "bytes=0-1048575", "")
	if err != nil {
		return mp4Index{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return mp4Index{}, errors.New("无法读取平台媒体索引")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1048576))
	if err != nil {
		return mp4Index{}, errors.New("媒体索引读取失败")
	}
	return readMP4Index(data)
}

func xmlText(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func (s *Server) platformMPD(ctx context.Context, raw string, ticket MediaTicket) ([]byte, error) {
	if ticket.Duration <= 0 || ticket.Duration > maxWatchSeconds || ticket.AudioURL == "" {
		return nil, errors.New("平台没有提供有效的音视频信息")
	}
	videoIndex, err := s.sources.probeMP4Index(ctx, ticket, ticket.StreamURL)
	if err != nil {
		return nil, err
	}
	audioIndex, err := s.sources.probeMP4Index(ctx, ticket, ticket.AudioURL)
	if err != nil {
		return nil, err
	}
	videoURL, err := s.platformAssetURL(raw, ticket.StreamURL, false)
	if err != nil {
		return nil, err
	}
	audioURL, err := s.platformAssetURL(raw, ticket.AudioURL, false)
	if err != nil {
		return nil, err
	}
	representation := func(id, mime, codecs, resource string, index mp4Index, bandwidth int64) string {
		return fmt.Sprintf(`<AdaptationSet mimeType="%s" segmentAlignment="true"><Representation id="%s" bandwidth="%d" codecs="%s"><BaseURL>%s</BaseURL><SegmentBase indexRange="%d-%d"><Initialization range="0-%d"/></SegmentBase></Representation></AdaptationSet>`, mime, id, max(bandwidth, 128000), xmlText(codecs), xmlText(resource), index.indexStart, index.indexEnd, index.initEnd)
	}
	document := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" profiles="urn:mpeg:dash:profile:isoff-on-demand:2011" mediaPresentationDuration="PT%.3fS" minBufferTime="PT1.5S"><Period duration="PT%.3fS">%s%s</Period></MPD>`, ticket.Duration, ticket.Duration,
		representation("video", "video/mp4", ticket.VideoCodec, videoURL, videoIndex, ticket.Bandwidth), representation("audio", "audio/mp4", ticket.AudioCodec, audioURL, audioIndex, 128000))
	return []byte(document), nil
}

func (s *Server) proxyPlatformManifest(w http.ResponseWriter, r *http.Request, raw string, ticket MediaTicket) bool {
	if ticket.Kind == "platform-dash" {
		data, err := s.platformMPD(r.Context(), raw, ticket)
		if err != nil {
			writeError(w, 502, err)
			return true
		}
		w.Header().Set("Content-Type", "application/dash+xml")
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
		return true
	}
	if ticket.Kind != "platform-hls" {
		return false
	}
	response, err := s.sources.Open(r.Context(), ticket, http.MethodGet, "", "")
	if err != nil {
		writeError(w, 502, errors.New("平台播放清单暂时不可用"))
		return true
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		writeError(w, 502, errors.New("平台播放清单已失效，请刷新片源"))
		return true
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil {
		writeError(w, 502, errors.New("播放清单读取失败"))
		return true
	}
	base := ticket.StreamURL
	if response.Request != nil && response.Request.URL != nil {
		base = response.Request.URL.String()
	}
	data, err = s.rewriteHLS(raw, base, data)
	if err != nil {
		writeError(w, 502, err)
		return true
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
	return true
}
