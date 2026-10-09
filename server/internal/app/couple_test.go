package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCoupleBindingMomentsCoolingAndRestore(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	tokens, _ := NewTokenManager("couple-test-secret-with-at-least-32-characters", "couple-test")
	auth := NewAuthService(repo, tokens)
	aDev := DeviceInfo{ID: "couple-user-a-device", Label: "A", Platform: "test"}
	bDev := DeviceInfo{ID: "couple-user-b-device", Label: "B", Platform: "test"}
	a, err := auth.Register(ctx, "a@couple.test", "Alice", "correct horse battery", aDev)
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.Register(ctx, "b@couple.test", "Bob", "another strong password", bDev)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}, Repository: repo, Auth: auth})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	status := requestStatusDevice(t, http.MethodPost, httpServer.URL+"/api/v1/couple/requests", a.AccessToken, map[string]string{"user_id": b.User.ID}, aDev)
	if status != http.StatusForbidden {
		t.Fatalf("non-vip request=%d", status)
	}
	if err := repo.SetUserVIP(ctx, a.User.ID, time.Now().Add(30*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	requestBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/couple/requests", a.AccessToken, map[string]string{"user_id": b.User.ID}, aDev)
	var request struct {
		Data CoupleRequest `json:"data"`
	}
	_ = json.Unmarshal(requestBody, &request)
	if request.Data.ID == 0 {
		t.Fatalf("request=%+v", request)
	}
	requestsBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/couple/requests", b.AccessToken, nil, bDev)
	var requests struct {
		Data struct {
			Requests []CoupleRequest `json:"requests"`
		} `json:"data"`
	}
	_ = json.Unmarshal(requestsBody, &requests)
	if len(requests.Data.Requests) != 1 {
		t.Fatalf("requests=%+v", requests)
	}
	acceptBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/couple/requests/"+strconvFormat(request.Data.ID)+"/respond", b.AccessToken, map[string]bool{"accept": true}, bDev)
	var accepted struct {
		Data Couple `json:"data"`
	}
	_ = json.Unmarshal(acceptBody, &accepted)
	if accepted.Data.Status != "active" || accepted.Data.Partner.ID != a.User.ID {
		t.Fatalf("couple=%+v", accepted.Data)
	}
	momentBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/couple/moments", a.AccessToken, map[string]string{"body": "Our first movie"}, aDev)
	var moment struct {
		Data CoupleMoment `json:"data"`
	}
	_ = json.Unmarshal(momentBody, &moment)
	if moment.Data.Body != "Our first movie" {
		t.Fatalf("moment=%+v", moment)
	}
	timelineBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/couple/timeline", a.AccessToken, nil, aDev)
	var timeline struct {
		Data struct {
			Events []CoupleEvent `json:"events"`
		} `json:"data"`
	}
	_ = json.Unmarshal(timelineBody, &timeline)
	if len(timeline.Data.Events) < 2 {
		t.Fatalf("timeline=%+v", timeline)
	}
	separatedBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/couple/separate", a.AccessToken, nil, aDev)
	var separated struct {
		Data Couple `json:"data"`
	}
	_ = json.Unmarshal(separatedBody, &separated)
	if separated.Data.Status != "separated" || separated.Data.CoolingPeriodEnd <= time.Now().UnixMilli() {
		t.Fatalf("separated=%+v", separated.Data)
	}
	restoredBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/couple/restore", b.AccessToken, nil, bDev)
	var restored struct {
		Data Couple `json:"data"`
	}
	_ = json.Unmarshal(restoredBody, &restored)
	if restored.Data.Status != "active" {
		t.Fatalf("restored=%+v", restored.Data)
	}
}
