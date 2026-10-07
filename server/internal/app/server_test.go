package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndJoinRoomAPI(t *testing.T) {
	server := NewServer(Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}, AllowDemoAuth: true,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	owner := createTestSession(t, httpServer.URL, "Owner")
	member := createTestSession(t, httpServer.URL, "Member")
	body := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/rooms", owner.AccessToken, map[string]any{
		"name": "Friday movie", "source_url": "https://example.com/movie.mp4", "max_members": 4,
	})
	var created struct {
		Data Room `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Data.Code) != 6 {
		t.Fatalf("room code = %q", created.Data.Code)
	}
	joinedBody := requestJSON(t, http.MethodPost, httpServer.URL+"/api/v1/rooms/"+created.Data.Code+"/join", member.AccessToken, nil)
	var joined struct {
		Data Room `json:"data"`
	}
	if err := json.Unmarshal(joinedBody, &joined); err != nil {
		t.Fatal(err)
	}
	if len(joined.Data.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(joined.Data.Members))
	}
}

func createTestSession(t *testing.T, baseURL, name string) Session {
	t.Helper()
	body := requestJSON(t, http.MethodPost, baseURL+"/api/v1/session/demo", "", map[string]string{"display_name": name})
	var response struct {
		Data Session `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func requestJSON(t *testing.T, method, target, token string, payload any) []byte {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, _ := json.Marshal(payload)
		body = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequest(method, target, body)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		t.Fatalf("%s %s: status=%d body=%s", method, target, response.StatusCode, data)
	}
	return data
}
