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

func TestAccountSecurityEndpoints(t *testing.T) {
	repository := NewMemoryRepository()
	tokens, _ := NewTokenManager("endpoint-security-secret-with-at-least-32-characters", "sameframe-endpoint-test")
	mailer := &MemoryMailer{}
	server := NewServer(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"*"}, Repository: repository,
		Auth: NewAuthService(repository, tokens, mailer),
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	deviceA := DeviceInfo{ID: "endpoint-device-a-0001", Label: "Laptop", Platform: "windows"}
	deviceB := DeviceInfo{ID: "endpoint-device-b-0002", Label: "Browser", Platform: "web"}

	registeredBody := requestJSONDevice(
		t,
		http.MethodPost,
		httpServer.URL+"/api/v1/auth/register",
		"",
		map[string]string{
			"email": "endpoint@example.com", "display_name": "Endpoint User",
			"password": "endpoint good password",
		},
		deviceA,
	)
	var registered struct {
		Data Session `json:"data"`
	}
	if err := json.Unmarshal(registeredBody, &registered); err != nil {
		t.Fatal(err)
	}
	verification, ok := mailer.LastMessage()
	if !ok {
		t.Fatal("verification message missing")
	}
	requestJSONDevice(
		t,
		http.MethodPost,
		httpServer.URL+"/api/v1/auth/email/verify",
		"",
		map[string]string{"token": verification.Token},
		deviceA,
	)
	loginBody := requestJSONDevice(
		t,
		http.MethodPost,
		httpServer.URL+"/api/v1/auth/login",
		"",
		map[string]string{"email": "endpoint@example.com", "password": "endpoint good password"},
		deviceB,
	)
	var login struct {
		Data Session `json:"data"`
	}
	if err := json.Unmarshal(loginBody, &login); err != nil {
		t.Fatal(err)
	}
	devicesBody := requestJSONDevice(
		t,
		http.MethodGet,
		httpServer.URL+"/api/v1/devices",
		login.Data.AccessToken,
		nil,
		deviceB,
	)
	var devices struct {
		Data struct {
			Devices []UserDevice `json:"devices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(devicesBody, &devices); err != nil {
		t.Fatal(err)
	}
	if len(devices.Data.Devices) != 2 {
		t.Fatalf("devices=%+v", devices.Data.Devices)
	}
	deviceAHash := hashDeviceID(deviceA.ID)
	requestJSONDevice(
		t,
		http.MethodDelete,
		httpServer.URL+"/api/v1/devices/"+deviceAHash,
		login.Data.AccessToken,
		nil,
		deviceB,
	)
	status := requestStatusDevice(
		t,
		http.MethodGet,
		httpServer.URL+"/api/v1/users/me",
		registered.Data.AccessToken,
		nil,
		deviceA,
	)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked device status=%d, want 401", status)
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
	return requestJSONDevice(
		t, method, target, token, payload,
		DeviceInfo{ID: "test-device-0001", Label: "Test device", Platform: "test"},
	)
}

func requestJSONDevice(
	t *testing.T,
	method, target, token string,
	payload any,
	device DeviceInfo,
) []byte {
	t.Helper()
	response := requestDevice(t, method, target, token, payload, device)
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		t.Fatalf("%s %s: status=%d body=%s", method, target, response.StatusCode, data)
	}
	return data
}

func requestStatusDevice(
	t *testing.T,
	method, target, token string,
	payload any,
	device DeviceInfo,
) int {
	t.Helper()
	response := requestDevice(t, method, target, token, payload, device)
	defer response.Body.Close()
	return response.StatusCode
}

func requestDevice(
	t *testing.T,
	method, target, token string,
	payload any,
	device DeviceInfo,
) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, _ := json.Marshal(payload)
		body = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequest(method, target, body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Device-ID", device.ID)
	request.Header.Set("X-Device-Name", device.Label)
	request.Header.Set("X-Device-Platform", device.Platform)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
