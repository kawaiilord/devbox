package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersonalLibraryFavoritesHistoryPrivacyAndResume(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("personal-library-test-secret-at-least-32-characters", "personal-library-test")
	auth := NewAuthService(repository, tokens)
	ownerDevice := DeviceInfo{ID: "library-owner-device", Label: "Owner", Platform: "test"}
	memberDevice := DeviceInfo{ID: "library-member-device", Label: "Member", Platform: "test"}
	owner, err := auth.Register(ctx, "library-owner@example.test", "Owner", "correct horse battery", ownerDevice)
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "library-member@example.test", "Member", "another strong password", memberDevice)
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(repository, vault, true)
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sourceServer.Close()
	source, err := manager.CreateWebDAV(
		ctx, owner.User, "Library", sourceServer.URL+"/dav/", "viewer", "secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	seed := NewStore()
	room, err := seed.CreateMediaRoom(owner.User, "Shared Movie", source.ID, "/movie.mp4", 4)
	if err != nil {
		t.Fatal(err)
	}
	room, err = seed.JoinRoom(room.Code, member.User)
	if err != nil {
		t.Fatal(err)
	}
	position := 95.0
	room, _, err = seed.ApplyControl(room.Code, owner.User, 1, Control{Action: "seek", Position: &position})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"*"}, Repository: repository, Auth: auth,
		InitialRooms: []Room{room}, Sources: manager,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	favoriteBody := requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/favorites", owner.AccessToken,
		map[string]any{
			"source_id": source.ID, "media_path": "/movie.mp4", "title": "Movie",
			"content_type": "video/mp4", "size": 1024,
		}, ownerDevice,
	)
	var favoriteResponse struct {
		Data Favorite `json:"data"`
	}
	if err := json.Unmarshal(favoriteBody, &favoriteResponse); err != nil {
		t.Fatal(err)
	}
	if favoriteResponse.Data.ID == 0 || favoriteResponse.Data.SourceName != "Library" {
		t.Fatalf("favorite=%+v", favoriteResponse.Data)
	}
	foreignFavoriteStatus := requestStatusDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/favorites", member.AccessToken,
		map[string]any{
			"source_id": source.ID, "media_path": "/movie.mp4", "title": "Movie",
			"content_type": "video/mp4", "size": 1024,
		}, memberDevice,
	)
	if foreignFavoriteStatus != http.StatusNotFound {
		t.Fatalf("foreign source favorite status=%d, want 404", foreignFavoriteStatus)
	}
	foreignDeleteStatus := requestStatusDevice(
		t, http.MethodDelete,
		httpServer.URL+"/api/v1/favorites/"+strconvFormat(favoriteResponse.Data.ID),
		member.AccessToken, nil, memberDevice,
	)
	if foreignDeleteStatus != http.StatusNotFound {
		t.Fatalf("foreign favorite deletion status=%d, want 404", foreignDeleteStatus)
	}
	secondFavoriteBody := requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/favorites", owner.AccessToken,
		map[string]any{
			"source_id": source.ID, "media_path": "/movie.mp4", "title": "Movie renamed",
			"content_type": "video/mp4", "size": 2048,
		}, ownerDevice,
	)
	var secondFavorite struct {
		Data Favorite `json:"data"`
	}
	if err := json.Unmarshal(secondFavoriteBody, &secondFavorite); err != nil {
		t.Fatal(err)
	}
	if secondFavorite.Data.ID != favoriteResponse.Data.ID {
		t.Fatalf("favorite upsert allocated a new id: first=%+v second=%+v", favoriteResponse.Data, secondFavorite.Data)
	}

	watchBody := requestJSONDevice(
		t, http.MethodPut,
		httpServer.URL+"/api/v1/rooms/"+room.Code+"/watch-progress",
		owner.AccessToken, map[string]any{"duration_seconds": 100}, ownerDevice,
	)
	var watchResponse struct {
		Data WatchRecord `json:"data"`
	}
	if err := json.Unmarshal(watchBody, &watchResponse); err != nil {
		t.Fatal(err)
	}
	if !watchResponse.Data.Completed || !watchResponse.Data.Resumable ||
		watchResponse.Data.CompanionCount != 1 || watchResponse.Data.SourceID != source.ID {
		t.Fatalf("watch record=%+v", watchResponse.Data)
	}
	historyBody := requestJSONDevice(
		t, http.MethodGet, httpServer.URL+"/api/v1/history", owner.AccessToken, nil, ownerDevice,
	)
	var history struct {
		Data struct {
			Records []WatchRecord `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(historyBody, &history); err != nil ||
		len(history.Data.Records) != 1 || history.Data.Records[0].ID != watchResponse.Data.ID {
		t.Fatalf("history=%+v error=%v", history, err)
	}
	memberWatchBody := requestJSONDevice(
		t, http.MethodPut,
		httpServer.URL+"/api/v1/rooms/"+room.Code+"/watch-progress",
		member.AccessToken, map[string]any{"duration_seconds": 100}, memberDevice,
	)
	var memberWatch struct {
		Data WatchRecord `json:"data"`
	}
	if err := json.Unmarshal(memberWatchBody, &memberWatch); err != nil {
		t.Fatal(err)
	}
	if memberWatch.Data.Resumable || memberWatch.Data.SourceID != "" {
		t.Fatalf("member inherited owner's source access: %+v", memberWatch.Data)
	}

	activityBody := requestJSONDevice(
		t, http.MethodGet,
		httpServer.URL+"/api/v1/users/"+owner.User.ID+"/watch-activity",
		member.AccessToken, nil, memberDevice,
	)
	var activity struct {
		Data struct {
			Activity []WatchActivity `json:"activity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(activityBody, &activity); err != nil || len(activity.Data.Activity) != 1 {
		t.Fatalf("activity=%+v error=%v", activity, err)
	}
	if err := repository.UpdatePrivacy(ctx, owner.User.ID, PrivacySettings{
		AllowRoomChat: true, AllowProfileFind: true, ShowWatchActivity: false,
	}); err != nil {
		t.Fatal(err)
	}
	privateStatus := requestStatusDevice(
		t, http.MethodGet,
		httpServer.URL+"/api/v1/users/"+owner.User.ID+"/watch-activity",
		member.AccessToken, nil, memberDevice,
	)
	if privateStatus != http.StatusForbidden {
		t.Fatalf("private watch activity status=%d, want 403", privateStatus)
	}
	if err := repository.UpdatePrivacy(ctx, owner.User.ID, PrivacySettings{
		AllowRoomChat: true, AllowProfileFind: true, ShowWatchActivity: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.BlockUser(ctx, member.User.ID, owner.User.ID); err != nil {
		t.Fatal(err)
	}
	blockedStatus := requestStatusDevice(
		t, http.MethodGet,
		httpServer.URL+"/api/v1/users/"+owner.User.ID+"/watch-activity",
		member.AccessToken, nil, memberDevice,
	)
	if blockedStatus != http.StatusForbidden {
		t.Fatalf("blocked watch activity status=%d, want 403", blockedStatus)
	}

	resumeBody := requestJSONDevice(
		t, http.MethodPost, httpServer.URL+"/api/v1/rooms", owner.AccessToken,
		map[string]any{
			"name": "Resume room", "source_url": "https://example.com/movie.mp4",
			"max_members": 4, "start_position": 42.5,
		}, ownerDevice,
	)
	var resumed struct {
		Data Room `json:"data"`
	}
	if err := json.Unmarshal(resumeBody, &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Data.Playback.Position != 42.5 {
		t.Fatalf("resume position=%v", resumed.Data.Playback.Position)
	}

	requestJSONDevice(
		t, http.MethodDelete,
		httpServer.URL+"/api/v1/favorites/"+strconvFormat(favoriteResponse.Data.ID),
		owner.AccessToken, nil, ownerDevice,
	)
	favoritesBody := requestJSONDevice(
		t, http.MethodGet, httpServer.URL+"/api/v1/favorites", owner.AccessToken, nil, ownerDevice,
	)
	var favorites struct {
		Data struct {
			Favorites []Favorite `json:"favorites"`
		} `json:"data"`
	}
	if err := json.Unmarshal(favoritesBody, &favorites); err != nil || len(favorites.Data.Favorites) != 0 {
		t.Fatalf("favorites=%+v error=%v", favorites, err)
	}
	requestJSONDevice(
		t, http.MethodDelete,
		httpServer.URL+"/api/v1/history/"+strconvFormat(watchResponse.Data.ID),
		owner.AccessToken, nil, ownerDevice,
	)
}

func TestWatchCompletionAndCompanionCountAreMonotonic(t *testing.T) {
	repository := NewMemoryRepository()
	first, err := repository.UpsertWatchRecord(context.Background(), WatchRecord{
		UserID: "user", MediaKey: strings.Repeat("b", 64), Title: "Movie",
		Position: 100, Duration: 100, Completed: true, CompanionCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.UpsertWatchRecord(context.Background(), WatchRecord{
		UserID: "user", MediaKey: strings.Repeat("b", 64), Title: "Movie",
		Position: 10, Duration: 100, Completed: false, CompanionCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || !second.Completed || second.CompanionCount != 3 {
		t.Fatalf("monotonic watch record failed: first=%+v second=%+v", first, second)
	}
}
