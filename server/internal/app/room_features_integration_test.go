package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPlaylistSelectionIsSerializedAcrossRedisNodesAndPersists(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if redisURL == "" || databaseURL == "" {
		t.Skip("integration stores not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := OpenRedis(ctx, redisURL, "playlist-node-a")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenRedis(ctx, redisURL, "playlist-node-b")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	tokens, _ := NewTokenManager("playlist-integration-secret-more-than-32-characters", "playlist-integration")
	auth := NewAuthService(repo, tokens)
	owner, err := auth.Register(ctx, "playlist-"+mustRandomString(8)+"@example.com", "Owner", "strong playlist password")
	if err != nil {
		t.Fatal(err)
	}
	serverA := NewServer(Options{Repository: repo, Auth: auth, Redis: a})
	serverB := NewServer(Options{Repository: repo, Auth: auth, Redis: b})
	serverA.StartBackground(ctx)
	serverB.StartBackground(ctx)
	httpA := httptest.NewServer(serverA.Handler())
	defer httpA.Close()
	httpB := httptest.NewServer(serverB.Handler())
	defer httpB.Close()
	room := decodeFeatureRoom(t, requestJSON(t, "POST", httpA.URL+"/api/v1/rooms", owner.AccessToken, map[string]string{"name": "Episodes", "source_url": "https://example.com/one.mp4"}))
	defer repo.pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", owner.User.ID)
	defer repo.pool.Exec(context.Background(), "DELETE FROM rooms WHERE code=$1", room.Code)
	room = decodeFeatureRoom(t, requestJSON(t, "POST", httpA.URL+"/api/v1/rooms/"+room.Code+"/playlist", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "items": []any{map[string]any{"title": "Second", "sources": []any{map[string]string{"url": "https://example.com/two.mp4"}}}}}))
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for i, base := range []string{httpA.URL, httpB.URL} {
		wg.Add(1)
		go func(i int, base string) {
			defer wg.Done()
			statuses <- requestStatusDevice(t, "POST", base+"/api/v1/rooms/"+room.Code+"/playback/select", owner.AccessToken, map[string]any{"expected_version": room.Features.Version, "item_id": room.Features.Playlist[i].ID}, DeviceInfo{ID: "test-device-0001", Label: "Test", Platform: "test"})
		}(i, base)
	}
	wg.Wait()
	close(statuses)
	ok, conflict := 0, 0
	for status := range statuses {
		if status == 200 {
			ok++
		}
		if status == 409 {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("concurrent selects success=%d conflict=%d", ok, conflict)
	}
	f, err := repo.GetRoomFeatures(ctx, room.Code)
	if err != nil {
		t.Fatal(err)
	}
	selected, _, found := f.activeSource()
	if !found {
		t.Fatal("selected source missing")
	}
	cached, _, err := b.Room(ctx, room.Code)
	if err != nil {
		t.Fatal(err)
	}
	if cached.SourceURL != selected.URL || cached.Playback.SourceVersion != 2 {
		t.Fatalf("mixed Redis media state: %+v", cached)
	}
	persisted, err := repo.LoadRooms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, r := range persisted {
		if r.Code == room.Code {
			matched = true
			if r.SourceURL != selected.URL || r.Playback.SourceVersion != 2 {
				t.Fatal("SQL selection was not atomic")
			}
		}
	}
	if !matched {
		t.Fatal("room disappeared")
	}
	if err = repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.GetRoomFeatures(ctx, room.Code)
	if after.Version != f.Version {
		t.Fatal("legacy backfill overwrote an existing playlist")
	}
	encoded, _ := json.Marshal(f.view(cached, owner.User.ID))
	if len(encoded) == 0 {
		t.Fatal("public playlist unavailable")
	}
}
