package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeObjectStore struct {
	mu      sync.Mutex
	objects map[string]ObjectInfo
	deleted []string
}

func TestS3ObjectStoreConfigurationValidation(t *testing.T) {
	tests := []string{
		"s3.example.com",
		"ftp://s3.example.com",
		"https://user:secret@s3.example.com",
		"https://s3.example.com/path",
		"https://s3.example.com?region=test",
	}
	for _, endpoint := range tests {
		if _, err := NewS3ObjectStore(endpoint, "access", "secret", "bucket"); err == nil {
			t.Fatalf("endpoint %q should be rejected", endpoint)
		}
	}
	store, err := NewS3ObjectStore("https://s3.example.com", "access", "secret", "bucket")
	if err != nil || store == nil {
		t.Fatalf("valid endpoint failed: %v", err)
	}
}

func (f *fakeObjectStore) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.test/put/" + key, nil
}
func (f *fakeObjectStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.test/get/" + key, nil
}
func (f *fakeObjectStore) Stat(_ context.Context, key string) (ObjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.objects[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return value, nil
}
func (f *fakeObjectStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}
func TestReviewsPresignedImagesCommentsAndBlocks(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	tokens, _ := NewTokenManager("reviews-test-secret-with-at-least-32-characters", "reviews-test")
	auth := NewAuthService(repo, tokens)
	aDev := DeviceInfo{ID: "review-user-a-device", Label: "A", Platform: "test"}
	bDev := DeviceInfo{ID: "review-user-b-device", Label: "B", Platform: "test"}
	a, err := auth.Register(ctx, "a@reviews.test", "Alice", "correct horse battery", aDev)
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.Register(ctx, "b@reviews.test", "Bob", "another strong password", bDev)
	if err != nil {
		t.Fatal(err)
	}
	objects := &fakeObjectStore{objects: map[string]ObjectInfo{}}
	server := NewServer(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}, Repository: repo, Auth: auth, Objects: objects})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	uploadBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/reviews/uploads", a.AccessToken, map[string]any{"filename": "cover.jpg", "content_type": "image/jpeg", "size": 100}, aDev)
	var upload struct {
		Data ObjectUpload `json:"data"`
	}
	_ = json.Unmarshal(uploadBody, &upload)
	if !strings.HasPrefix(upload.Data.ObjectKey, "reviews/"+a.User.ID+"/") {
		t.Fatalf("upload=%+v", upload.Data)
	}
	objects.objects[upload.Data.ObjectKey] = ObjectInfo{Size: 100, ContentType: "image/jpeg"}
	reviewBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/reviews", a.AccessToken, map[string]any{"target_type": "movie", "target_id": "42", "title": "Movie", "rating": 9, "content": "Excellent", "image_keys": []string{upload.Data.ObjectKey}}, aDev)
	var review struct {
		Data Review `json:"data"`
	}
	_ = json.Unmarshal(reviewBody, &review)
	if review.Data.ID == 0 || len(review.Data.ImageURLs) != 1 {
		t.Fatalf("review=%+v", review.Data)
	}
	requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/reviews", a.AccessToken, map[string]any{"target_type": "movie", "target_id": "42", "title": "Movie", "rating": 8, "content": "Updated", "image_keys": []string{}}, aDev)
	if len(objects.deleted) != 1 || objects.deleted[0] != upload.Data.ObjectKey {
		t.Fatalf("replaced image was not deleted: %v", objects.deleted)
	}
	commentBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/reviews/"+strconvFormat(review.Data.ID)+"/comments", b.AccessToken, map[string]string{"body": "Agreed"}, bDev)
	var comment struct {
		Data ReviewComment `json:"data"`
	}
	_ = json.Unmarshal(commentBody, &comment)
	if comment.Data.Body != "Agreed" {
		t.Fatalf("comment=%+v", comment)
	}
	listBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/reviews?target_type=movie&target_id=42", b.AccessToken, nil, bDev)
	var list struct {
		Data struct {
			Reviews []Review `json:"reviews"`
		} `json:"data"`
	}
	_ = json.Unmarshal(listBody, &list)
	if len(list.Data.Reviews) != 1 {
		t.Fatalf("reviews=%+v", list)
	}
	_ = repo.BlockUser(ctx, b.User.ID, a.User.ID)
	listBody = requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/reviews?target_type=movie&target_id=42", b.AccessToken, nil, bDev)
	_ = json.Unmarshal(listBody, &list)
	if len(list.Data.Reviews) != 0 {
		t.Fatalf("blocked reviews=%+v", list)
	}
	requestJSONDevice(t, http.MethodDelete, httpServer.URL+"/api/v1/reviews/"+strconvFormat(review.Data.ID), a.AccessToken, nil, aDev)
	if len(objects.deleted) != 1 {
		t.Fatalf("deleted=%v", objects.deleted)
	}
}
