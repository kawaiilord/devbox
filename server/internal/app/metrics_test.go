package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReadinessAndProtectedMetrics(t *testing.T) {
	repo := NewMemoryRepository()
	server := NewServer(Options{Repository: repo, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), MetricsToken: "metrics-secret"})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	for _, path := range []string{"/healthz", "/readyz"} {
		response, err := http.Get(httpServer.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d", path, response.StatusCode)
		}
	}
	response, err := http.Get(httpServer.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unprotected metrics status=%d", response.StatusCode)
	}
	request, _ := http.NewRequest(http.MethodGet, httpServer.URL+"/metrics", nil)
	request.Header.Set("Authorization", "Bearer metrics-secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "sameframe_http_requests_total") {
		t.Fatalf("metrics status=%d body=%s", response.StatusCode, body)
	}
}
