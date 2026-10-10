package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func decodeFeatureRoom(t *testing.T, body []byte) Room {
	t.Helper()
	var response struct {
		Data Room `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func TestPublicRoomsPasswordGuestScopesAndPermissions(t *testing.T) {
	ctx := context.Background()
	server := NewServer(Options{AllowDemoAuth: true, AllowedOrigins: []string{"*"}})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	owner := createTestSession(t, httpServer.URL, "Owner")
	member := createTestSession(t, httpServer.URL, "Member")
	created := requestJSON(t, "POST", httpServer.URL+"/api/v1/rooms", owner.AccessToken, map[string]any{
		"name": "Cinema", "source_url": "https://example.com/private-video.mp4?secret=source-only", "settings": map[string]any{"visibility": "public", "allow_guests": true, "password": "room-secret", "category": "电影", "tags": []string{"周末"}},
	})
	room := decodeFeatureRoom(t, created)
	if room.Features == nil || !room.Features.PasswordProtected || room.Features.Version != 1 {
		t.Fatal("room settings missing")
	}
	public := requestJSON(t, "GET", httpServer.URL+"/api/v1/public/rooms?q=Cinema", "", nil)
	if strings.Contains(string(public), "room-secret") || strings.Contains(string(public), "source-only") || strings.Contains(string(public), "argon2") {
		t.Fatal("public discovery exposed private room data")
	}
	var discovery struct {
		Data struct {
			Rooms []PublicRoom `json:"rooms"`
		} `json:"data"`
	}
	_ = json.Unmarshal(public, &discovery)
	if len(discovery.Data.Rooms) != 1 || !discovery.Data.Rooms[0].AllowGuests {
		t.Fatal("public discovery failed")
	}
	device := DeviceInfo{ID: "test-device-0001", Label: "Test device", Platform: "test"}
	joinURL := httpServer.URL + "/api/v1/rooms/" + room.Code + "/join"
	if status := requestStatusDevice(t, "POST", joinURL, member.AccessToken, map[string]string{"password": "wrong"}, device); status < 400 {
		t.Fatal("wrong room password accepted")
	}
	requestJSON(t, "POST", joinURL, member.AccessToken, map[string]string{"password": "room-secret"})
	guestBody := requestJSON(t, "POST", httpServer.URL+"/api/v1/rooms/"+room.Code+"/guest", "", map[string]string{"display_name": "Guest viewer", "password": "room-secret"})
	var guest struct {
		Data struct {
			Session Session `json:"session"`
			Room    Room    `json:"room"`
		} `json:"data"`
	}
	if err := json.Unmarshal(guestBody, &guest); err != nil {
		t.Fatal(err)
	}
	if guest.Data.Session.RefreshToken != "" || guest.Data.Session.User.GuestRoomCode != room.Code || guest.Data.Room.Features.Permissions.Playback {
		t.Fatal("guest received unrestricted identity or permissions")
	}
	if status := requestStatusDevice(t, "GET", httpServer.URL+"/api/v1/devices", guest.Data.Session.AccessToken, nil, device); status < 400 {
		t.Fatal("guest escaped room scope")
	}
	requestJSON(t, "GET", httpServer.URL+"/api/v1/rooms/"+room.Code, guest.Data.Session.AccessToken, nil)
	if err := server.processPlaybackControl(ctx, room.Code, guest.Data.Session.User, 1, Control{Action: "play"}); err == nil {
		t.Fatal("watch-only guest controlled playback")
	}
	room = decodeFeatureRoom(t, requestJSON(t, "GET", httpServer.URL+"/api/v1/rooms/"+room.Code, owner.AccessToken, nil))
	room = decodeFeatureRoom(t, requestJSON(t, "PUT", httpServer.URL+"/api/v1/rooms/"+room.Code+"/members/"+member.User.ID, owner.AccessToken, map[string]any{
		"expected_version": room.Features.Version, "role": "moderator", "permissions": map[string]bool{"playback": true, "playlist": false, "chat": true, "danmaku": false, "voice": false},
	}))
	if err := server.processPlaybackControl(ctx, room.Code, member.User, 2, Control{Action: "play"}); err != nil {
		t.Fatalf("granted playback permission failed: %v", err)
	}
	if status := requestStatusDevice(t, "POST", httpServer.URL+"/api/v1/rooms/"+room.Code+"/playlist", member.AccessToken, map[string]any{"expected_version": room.Features.Version, "items": []any{map[string]any{"title": "Not allowed", "sources": []any{map[string]string{"url": "https://example.com/2.mp4"}}}}}, device); status != 403 {
		t.Fatalf("playlist deny status=%d", status)
	}
	room = decodeFeatureRoom(t, requestJSON(t, "PUT", httpServer.URL+"/api/v1/rooms/"+room.Code+"/settings", owner.AccessToken, map[string]any{
		"expected_version": room.Features.Version, "visibility": "private", "allow_guests": false, "description": "Private now",
	}))
	if status := requestStatusDevice(t, "GET", httpServer.URL+"/api/v1/rooms/"+room.Code, guest.Data.Session.AccessToken, nil, device); status < 400 {
		t.Fatal("disabled guest retained room access")
	}
	f, err := server.repo.GetRoomFeatures(ctx, room.Code)
	if err != nil || f.PasswordHash == "" || strings.Contains(f.PasswordHash, "room-secret") {
		t.Fatal("room password was not hashed or was lost")
	}
	public = requestJSON(t, "GET", httpServer.URL+"/api/v1/public/rooms", "", nil)
	_ = json.Unmarshal(public, &discovery)
	if len(discovery.Data.Rooms) != 0 {
		t.Fatal("private room appeared in discovery")
	}
}

func TestPlaylistEpisodesAlternativesReorderAndStaleControls(t *testing.T) {
	server := NewServer(Options{AllowDemoAuth: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	owner := createTestSession(t, httpServer.URL, "Owner")
	room := decodeFeatureRoom(t, requestJSON(t, "POST", httpServer.URL+"/api/v1/rooms", owner.AccessToken, map[string]string{"name": "Series", "source_url": "https://example.com/one.mp4"}))
	base := httpServer.URL + "/api/v1/rooms/" + room.Code
	room = decodeFeatureRoom(t, requestJSON(t, "POST", base+"/playlist", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "items": []any{
		map[string]any{"title": "第二集", "sources": []any{map[string]string{"url": "https://example.com/two.mp4", "label": "源 A"}, map[string]string{"url": "https://example.com/two-backup.mp4", "label": "源 B"}}},
		map[string]any{"title": "第三集", "sources": []any{map[string]string{"url": "https://example.com/three.mp4"}}},
	}}))
	if len(room.Features.Playlist) != 3 {
		t.Fatal("batch playlist addition failed")
	}
	second := room.Features.Playlist[1]
	room = decodeFeatureRoom(t, requestJSON(t, "POST", base+"/playback/select", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "item_id": second.ID}))
	if room.SourceURL != "https://example.com/two.mp4" || room.Playback.Episode != second.Episode || !room.Playback.Playing {
		t.Fatal("episode switch did not replace the media")
	}
	position := 37.0
	if err := server.processPlaybackControl(context.Background(), room.Code, owner.User, 10, Control{Action: "seek", Position: &position}); err != nil {
		t.Fatal(err)
	}
	previousVersion := room.Playback.SourceVersion
	room = decodeFeatureRoom(t, requestJSON(t, "POST", base+"/playback/select", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "item_id": second.ID, "source_id": second.Sources[1].ID}))
	if room.SourceURL != "https://example.com/two-backup.mp4" || room.Playback.Position < 37 || room.Playback.SourceVersion <= previousVersion {
		t.Fatal("source switch failed to preserve the position or advance version")
	}
	if err := server.processPlaybackControl(context.Background(), room.Code, owner.User, 11, Control{Action: "pause", SourceVersion: &previousVersion}); err == nil {
		t.Fatal("old player command overrode new source")
	}
	order := []string{room.Features.Playlist[2].ID, second.ID, room.Features.Playlist[0].ID}
	room = decodeFeatureRoom(t, requestJSON(t, "PUT", base+"/playlist", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "order": order}))
	if room.Features.Playlist[0].Title != "第三集" || room.Features.ActiveItemID != second.ID {
		t.Fatal("reorder changed the selected episode")
	}
	room = decodeFeatureRoom(t, requestJSON(t, "POST", base+"/playback/select", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "direction": "previous"}))
	if room.SourceURL != "https://example.com/three.mp4" || room.Playback.Position > 1 {
		t.Fatal("previous episode did not reset position")
	}
	for len(room.Features.Playlist) > 0 {
		room = decodeFeatureRoom(t, requestJSON(t, "DELETE", base+"/playlist/"+room.Features.Playlist[0].ID, owner.AccessToken, map[string]any{"expected_version": room.Features.Version}))
	}
	if room.SourceURL != "" || room.Playback.Playing || room.Features.ActiveItemID != "" {
		t.Fatal("empty playlist left media playing")
	}
	if room.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("playlist operations unexpectedly expired the room")
	}
}
