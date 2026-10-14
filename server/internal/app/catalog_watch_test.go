package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCatalogSoloTicketAndOwnedRoomHandoff(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("integration Redis not configured")
	}
	ctx := context.Background()
	f, c := catalogFixture(t)
	redis, err := OpenRedis(ctx, redisURL, "solo-catalog-test")
	if err != nil {
		t.Fatal(err)
	}
	defer redis.Close()
	redis.prefix = "solo-test-" + mustRandomString(8)
	f.server.redis = redis
	src := installTestCatalog(t, f, c)
	c.client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "cms.example" {
			return discoveryReply(`{"code":1,"pagecount":1,"list":[{"vod_id":12,"vod_name":"单人样本","type_name":"电视剧","vod_play_from":"主线","vod_play_url":"第1集$https://media.example/1.m3u8#第2集$https://media.example/2.m3u8"}]}`), nil
		}
		return discoveryReply("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nclip.ts\n#EXT-X-ENDLIST"), nil
	})
	film, err := c.detail(ctx, src, "12")
	if err != nil {
		t.Fatal(err)
	}
	selection := CatalogSelection{Refs: []CatalogRef{{SourceID: src.ID, FilmID: "12", GroupID: film.Groups[0].ID}}, EpisodeID: film.Groups[0].Episodes[1].ID, AllEpisodes: true}
	f.call(t, "POST", "/api/v1/catalog/watch", selection, "", 401)
	data := f.call(t, "POST", "/api/v1/catalog/watch", selection, f.member.AccessToken, 200)
	var view struct {
		URL string `json:"url"`
	}
	json.Unmarshal(data, &view)
	if !strings.HasPrefix(view.URL, "/media/") {
		t.Fatal("missing solo ticket")
	}
	raw := strings.Split(strings.TrimPrefix(view.URL, "/media/"), "/")[0]
	ticket, err := redis.MediaTicket(ctx, raw)
	if err != nil || ticket.RoomCode != "" || ticket.ViewerID != f.member.User.ID || !strings.Contains(ticket.StreamURL, "/2.m3u8") {
		t.Fatal("solo ticket is not viewer/episode bound", ticket, err)
	}
	_, total, err := f.repo.AdminFindRooms(ctx, AdminQuery{Limit: 20})
	if err != nil || total != 0 {
		t.Fatal("solo playback created room", total, err)
	}
	w := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(w, httptest.NewRequest("GET", view.URL, nil))
	if w.Code != 200 {
		t.Fatal("solo proxy failed", w.Code, w.Body.String())
	}
	roomData := f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "我的旧房间", "source_url": "https://media.example/old.mp4"}, f.member.AccessToken, 201)
	var room Room
	json.Unmarshal(roomData, &room)
	f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "其他人的房间", "source_url": "https://media.example/old.mp4"}, f.root.AccessToken, 201)
	f.call(t, "GET", "/api/v1/account/rooms", nil, "", 401)
	own := f.call(t, "GET", "/api/v1/account/rooms?owner_id="+f.root.User.ID, nil, f.member.AccessToken, 200)
	var list struct {
		Rooms []struct {
			Code string `json:"code"`
		} `json:"rooms"`
	}
	json.Unmarshal(own, &list)
	if len(list.Rooms) != 1 || list.Rooms[0].Code != room.Code || strings.Contains(string(own), "其他人的") {
		t.Fatal("owner room isolation failed", string(own))
	}
	changed := f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/catalog", map[string]any{"version": room.Features.Version, "selection": selection, "play_now": true, "start_position": 12.5}, f.member.AccessToken, 200)
	var after Room
	json.Unmarshal(changed, &after)
	if len(after.Features.Playlist) != 3 || after.Features.ActiveItemID != after.Features.Playlist[2].ID || after.Playback.Position < 12.5 || after.Playback.Position > 15 {
		t.Fatal("room handoff lost episode or position", after.Playback)
	}
	f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/catalog", map[string]any{"version": room.Features.Version, "selection": selection, "play_now": true}, f.member.AccessToken, 409)
	f.call(t, "POST", "/api/v1/rooms/"+room.Code+"/catalog", map[string]any{"version": after.Features.Version, "selection": selection, "play_now": true}, f.root.AccessToken, 403)
	record, err := f.repo.AdminReadRoom(ctx, room.Code)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.server.changeManagedRoom(ctx, room.Code, f.root.User, AdminRoomChange{Version: record.Features.Version, Action: "delete", Reason: "验收清理"})
	if err != nil {
		t.Fatal(err)
	}
	own = f.call(t, "GET", "/api/v1/account/rooms", nil, f.member.AccessToken, 200)
	json.Unmarshal(own, &list)
	if len(list.Rooms) != 0 {
		t.Fatal("closed room returned as available")
	}
	// Revoking the account invalidates the solo ticket without a room lookup.
	account, err := f.repo.UserByID(ctx, f.member.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	managed := managedUser(account)
	_, err = f.repo.AdminMutateUser(ctx, f.member.User.ID, f.root.User.ID, AdminUserChange{Version: managed.Version, Action: "ban", Reason: "ticket revocation test"})
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	f.server.Handler().ServeHTTP(w, httptest.NewRequest("GET", view.URL, nil))
	if w.Code != 401 {
		t.Fatal("revoked account retained solo ticket", w.Code)
	}
}
