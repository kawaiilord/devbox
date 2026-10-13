// catalog-fixture serves only a loopback, isolated acceptance environment.
// Production never imports its synthetic catalogs, transport, or test users.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kawaiilord/devbox/server/internal/app"
)

type fixtureTransport struct{ dir string }

func (t fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	header := http.Header{}
	var data []byte
	status := 200
	switch r.URL.Hostname() {
	case "fixture-catalog.example":
		switch r.URL.Path {
		case "/subscribe.json":
			data = []byte(`{"sites":[{"name":"同帧演示影视","type":1,"api":"https://fixture-catalog.example/api"},{"name":"备用演示源","type":1,"api":"https://fixture-catalog.example/backup"},{"name":"重复接口","type":1,"api":"https://fixture-catalog.example/api"},{"name":"脚本跳过示例","type":3,"api":"csp_demo"}],"lives":[{"name":"演示频道","url":"https://fixture-catalog.example/channels.m3u"}]}`)
		case "/channels.m3u":
			data = []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"sample\" group-title=\"测试直播\",同帧演示频道\nhttps://fixture-media.example/demo.m3u8\n")
		case "/api", "/backup":
			names := []string{"同帧测试电影", "星海旅人", "漫长的夏日", "城市的回声", "月光来信", "第七次日出", "夏日慢行", "时光放映室"}
			list := []map[string]any{}
			for i, name := range names {
				id := fmt.Sprint(i + 1)
				if v := r.URL.Query().Get("ids"); v != "" && v != id {
					continue
				}
				if v := r.URL.Query().Get("wd"); v != "" && !strings.Contains(name, v) {
					continue
				}
				play := "正片$https://fixture-media.example/demo.m3u8"
				kind := "电影"
				if i == 1 {
					kind = "电视剧"
					play = "第01集$https://fixture-media.example/demo.m3u8#第02集$https://fixture-media.example/demo.m3u8#第03集$https://fixture-media.example/demo.m3u8"
				}
				list = append(list, map[string]any{"vod_id": id, "vod_name": name, "vod_year": "2026", "type_name": kind, "vod_remarks": "隔离验收 · 演示片", "vod_pic": fmt.Sprintf("https://fixture-media.example/poster-%d.png", i), "vod_actor": "同帧演示片库", "vod_content": "此影片仅用于测试媒体订阅、选集、播放与房间同步。不会加入正式片库。", "vod_play_from": "主线$$$备用", "vod_play_url": play + "$$$" + strings.ReplaceAll(play, "demo.m3u8", "backup.m3u8")})
			}
			data, _ = json.Marshal(map[string]any{"code": 1, "pagecount": 1, "list": list})
		default:
			status = 404
		}
	case "fixture-media.example":
		if strings.HasPrefix(r.URL.Path, "/poster-") {
			img := image.NewRGBA(image.Rect(0, 0, 300, 450))
			seed := len(r.URL.Path) + int(r.URL.Path[len("/poster-")])
			for y := 0; y < 450; y++ {
				for x := 0; x < 300; x++ {
					shade := uint8((x+y+seed*19)%190 + 35)
					img.Set(x, y, color.RGBA{shade, uint8((y+seed*23)%130 + 40), uint8((x+seed*31)%150 + 50), 255})
				}
			}
			var b bytes.Buffer
			png.Encode(&b, img)
			data = b.Bytes()
			header.Set("Content-Type", "image/png")
		} else {
			p := filepath.Base(r.URL.Path)
			if p == "backup.m3u8" {
				p = "demo.m3u8"
			}
			var err error
			data, err = os.ReadFile(filepath.Join(t.dir, p))
			if err != nil {
				status = 404
			}
			if strings.HasSuffix(p, ".m3u8") {
				header.Set("Content-Type", "application/vnd.apple.mpegurl")
			} else {
				header.Set("Content-Type", "video/mp2t")
			}
		}
	default:
		return nil, fmt.Errorf("fixture allows only owned synthetic hosts")
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
}
func main() {
	credentials := flag.String("credentials", "", "private credentials file")
	media := flag.String("media", "", "directory of locally generated HLS")
	redisURL := flag.String("redis", "redis://127.0.0.1:16385/0", "isolated Redis")
	flag.Parse()
	if *credentials == "" || *media == "" {
		log.Fatal("credentials and media are required")
	}
	key := make([]byte, 32)
	rand.Read(key)
	secret := base64.StdEncoding.EncodeToString(key)
	password := "Fixture-" + secret[:24]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := app.NewMemoryRepository()
	tokens, _ := app.NewTokenManager(secret, "catalog-fixture")
	auth := app.NewAuthService(repo, tokens)
	device := app.DeviceInfo{ID: "catalog-fixture-device-0001", Label: "隔离测试", Platform: "test"}
	users := map[string]string{"email": "viewer@catalog.example.test", "admin_email": "admin@catalog.example.test", "peer_email": "peer@catalog.example.test", "password": password, "device": device.ID}
	for _, name := range []string{"email", "admin_email", "peer_email"} {
		session, e := auth.Register(ctx, users[name], "同帧测试", password, device)
		if e != nil {
			log.Fatal(e)
		}
		repo.VerifyEmail(ctx, session.User.ID)
		if name == "admin_email" {
			repo.SetUserAdmin(ctx, session.User.ID, true)
		}
	}
	raw, _ := json.Marshal(users)
	if e := os.WriteFile(*credentials, raw, 0600); e != nil {
		log.Fatal(e)
	}
	vault, _ := app.NewCredentialVault(secret)
	coordinator, e := app.OpenRedis(ctx, *redisURL, "catalog-fixture")
	if e != nil {
		log.Fatal(e)
	}
	defer coordinator.Close()
	sources := app.NewMediaSourceManager(repo, vault, false)
	server := app.NewServer(app.Options{Repository: repo, Auth: auth, Vault: vault, Sources: sources, Redis: coordinator, RequireAccount: true, CatalogTransport: fixtureTransport{*media}, AllowedOrigins: []string{"http://localhost:18883"}})
	server.StartBackground(ctx)
	log.Print("Catalog fixture on 127.0.0.1:18882; no production data")
	log.Fatal(http.ListenAndServe("127.0.0.1:18882", server.Handler()))
}
