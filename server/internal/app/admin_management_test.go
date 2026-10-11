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
	"time"
)

type managementFixture struct {
	repo                   Repository
	server                 *Server
	http                   *httptest.Server
	auth                   *AuthService
	root, member, operator Session
	device                 DeviceInfo
}

func newManagementFixture(t *testing.T, repo Repository) *managementFixture {
	t.Helper()
	ctx := context.Background()
	tokens, _ := NewTokenManager("admin-management-test-secret-with-more-than-32-characters", "admin-management-test")
	auth := NewAuthService(repo, tokens)
	device := DeviceInfo{ID: "management-test-device-0001", Label: "Test", Platform: "test"}
	root, err := auth.Register(ctx, "root@management.test", "Root", "correct horse battery", device)
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "member@management.test", "Member", "correct horse battery", device)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := auth.Register(ctx, "operator@management.test", "Operator", "correct horse battery", device)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.SetUserAdmin(ctx, root.User.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{root.User.ID, member.User.ID, operator.User.ID} {
		if err = repo.VerifyEmail(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err = repo.SetAdminRole(ctx, operator.User.ID, "operator", root.User.ID); err != nil {
		t.Fatal(err)
	}
	root, err = auth.Login(ctx, "root@management.test", "correct horse battery", device)
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	sources := NewMediaSourceManager(repo, vault, true)
	server := NewServer(Options{Repository: repo, Auth: auth, Sources: sources, Vault: vault, AllowedOrigins: []string{"*"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return &managementFixture{repo: repo, server: server, http: httpServer, auth: auth, root: root, member: member, operator: operator, device: device}
}
func (f *managementFixture) call(t *testing.T, method, path string, body any, token string, status int) json.RawMessage {
	t.Helper()
	raw, _ := json.Marshal(body)
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, f.http.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", f.device.ID)
	req.Header.Set("X-Device-Name", f.device.Label)
	req.Header.Set("X-Device-Platform", f.device.Platform)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if path == "/api/v1/admin/site-policy" {
		req.Header.Set("X-Confirm-Dangerous", "update-site-policy")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		t.Fatalf("%s %s status=%d want=%d: %s", method, path, res.StatusCode, status, payload)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(payload, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}
func (f *managementFixture) user(t *testing.T, id string) ManagedUser {
	t.Helper()
	a, e := f.repo.UserByID(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	return managedUser(a)
}
func (f *managementFixture) room(t *testing.T) AdminRoomRecord {
	t.Helper()
	raw := f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "Managed Cinema", "source_url": "https://example.com/video.mp4", "settings": map[string]any{"visibility": "public", "allow_guests": true}}, f.member.AccessToken, 201)
	var room Room
	if e := json.Unmarshal(raw, &room); e != nil {
		t.Fatal(e)
	}
	record, e := f.repo.AdminReadRoom(context.Background(), room.Code)
	if e != nil {
		t.Fatal(e)
	}
	return record
}
func TestAdminManagementAccountsRevocationAndBatchIsolation(t *testing.T) {
	testAdminManagementAccounts(t, NewMemoryRepository())
}
func testAdminManagementAccounts(t *testing.T, repo Repository) {
	f := newManagementFixture(t, repo)
	ctx := context.Background()
	target := f.user(t, f.member.User.ID)
	f.call(t, "GET", "/api/v1/admin/session", nil, f.operator.AccessToken, 200)
	f.call(t, "POST", "/api/v1/admin/managed-users", map[string]any{"email": "new@management.test", "display_name": "New User", "password": "another good password"}, f.operator.AccessToken, 403)
	raw := f.call(t, "POST", "/api/v1/admin/managed-users", map[string]any{"email": "new@management.test", "display_name": "New User", "password": "another good password", "email_verified": true}, f.root.AccessToken, 201)
	var created ManagedUser
	_ = json.Unmarshal(raw, &created)
	if !created.EmailVerified || created.ID == "" {
		t.Fatalf("created=%+v", created)
	}
	self := f.user(t, f.root.User.ID)
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+self.ID, map[string]any{"version": self.Version, "action": "ban", "reason": "test"}, f.root.AccessToken, 403)
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+target.ID, map[string]any{"version": target.Version + 1, "action": "ban", "reason": "test"}, f.root.AccessToken, 409)
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+target.ID, map[string]any{"version": target.Version, "action": "ban", "reason": "verified abuse"}, f.root.AccessToken, 200)
	f.call(t, "GET", "/api/v1/users/me", nil, f.member.AccessToken, 401)
	if _, err := f.auth.issueSession(ctx, f.member.User, hashDeviceID(f.device.ID)); err == nil {
		t.Fatal("stale login issued a new refresh token after ban")
	}
	if _, err := f.auth.Login(ctx, f.member.User.Email, "correct horse battery", f.device); err == nil {
		t.Fatal("banned user logged in")
	}
	if _, err := f.auth.Refresh(ctx, f.member.RefreshToken, f.device); err == nil {
		t.Fatal("banned refresh token rotated")
	}
	target = f.user(t, target.ID)
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+target.ID, map[string]any{"version": target.Version, "action": "unban"}, f.root.AccessToken, 200)
	f.call(t, "GET", "/api/v1/users/me", nil, f.member.AccessToken, 401)
	session, err := f.auth.Login(ctx, f.member.User.Email, "correct horse battery", f.device)
	if err != nil {
		t.Fatal(err)
	}
	target = f.user(t, target.ID)
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+target.ID, map[string]any{"version": target.Version, "action": "password", "password": "replacement password 123"}, f.root.AccessToken, 200)
	f.call(t, "GET", "/api/v1/users/me", nil, session.AccessToken, 401)
	if _, err = f.auth.Login(ctx, f.member.User.Email, "correct horse battery", f.device); err == nil {
		t.Fatal("old password still works")
	}
	if _, err = f.auth.Login(ctx, f.member.User.Email, "replacement password 123", f.device); err != nil {
		t.Fatal(err)
	}
	target = f.user(t, target.ID)
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+target.ID, map[string]any{"version": target.Version, "action": "delete", "reason": "test"}, f.root.AccessToken, 428)
	raw = f.call(t, "POST", "/api/v1/admin/managed-users/batch", map[string]any{"action": "delete", "reason": "test deletion", "confirm": "DELETE ACCOUNTS", "targets": []map[string]any{{"id": target.ID, "version": target.Version}, {"id": self.ID, "version": self.Version}}}, f.root.AccessToken, 200)
	var batch struct {
		Results []struct {
			ID string `json:"id"`
			OK bool   `json:"ok"`
		} `json:"results"`
	}
	_ = json.Unmarshal(raw, &batch)
	if len(batch.Results) != 2 || !batch.Results[0].OK || batch.Results[1].OK {
		t.Fatalf("batch=%s", raw)
	}
	target = f.user(t, target.ID)
	if target.Status != "deleted" {
		t.Fatal("account not in recycle bin")
	}
	f.call(t, "PUT", "/api/v1/admin/managed-users/"+target.ID, map[string]any{"version": target.Version, "action": "restore"}, f.root.AccessToken, 200)
	if f.user(t, target.ID).Status != "active" {
		t.Fatal("restore failed")
	}
	events, err := repo.ListAudit(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "replacement password") || strings.Contains(string(encoded), "argon2") {
		t.Fatal("password leaked to audit")
	}
}
func TestAdminManagementRoomsPolicyAndTaxonomy(t *testing.T) {
	testAdminManagementRooms(t, NewMemoryRepository())
}
func testAdminManagementRooms(t *testing.T, repo Repository) {
	f := newManagementFixture(t, repo)
	record := f.room(t)
	code := record.Room.Code
	path := "/api/v1/admin/rooms/" + code
	f.call(t, "GET", "/api/v1/admin/rooms", nil, f.operator.AccessToken, 200)
	f.call(t, "PUT", path, AdminRoomChange{Action: "ban", Version: record.Features.Version, Reason: "test"}, f.operator.AccessToken, 403)
	f.call(t, "PUT", path, AdminRoomChange{Action: "settings", Version: record.Features.Version, Name: "Renamed Cinema", MaxMembers: 10, Settings: roomSettingsInput{Visibility: "public", Category: "films", Tags: []string{"weekend"}, AllowGuests: true}}, f.root.AccessToken, 200)
	f.call(t, "PUT", path, AdminRoomChange{Action: "ban", Version: record.Features.Version, Reason: "stale"}, f.root.AccessToken, 409)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	f.call(t, "PUT", path, AdminRoomChange{Action: "member_add", Version: record.Features.Version, UserID: f.operator.User.ID}, f.root.AccessToken, 200)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	f.call(t, "PUT", path, AdminRoomChange{Action: "member_update", Version: record.Features.Version, UserID: f.operator.User.ID, Role: "moderator", Permissions: &RoomPermissions{Playback: true, Chat: false}}, f.root.AccessToken, 200)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	if record.Features.grants(record.Room, f.operator.User.ID).Chat {
		t.Fatal("member chat override ignored")
	}
	f.call(t, "PUT", path, AdminRoomChange{Action: "member_remove", Version: record.Features.Version, UserID: f.member.User.ID, Reason: "owner"}, f.root.AccessToken, 403)
	f.call(t, "PUT", path, AdminRoomChange{Action: "member_remove", Version: record.Features.Version, UserID: f.operator.User.ID, Reason: "moderation"}, f.root.AccessToken, 200)
	f.call(t, "POST", "/api/v1/rooms/"+code+"/join", map[string]string{}, f.operator.AccessToken, 403)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	f.call(t, "PUT", path, AdminRoomChange{Action: "ban", Version: record.Features.Version, Reason: "review"}, f.root.AccessToken, 200)
	f.call(t, "GET", "/api/v1/rooms/"+code, nil, f.member.AccessToken, 403)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	f.call(t, "PUT", path, AdminRoomChange{Action: "unban", Version: record.Features.Version}, f.root.AccessToken, 200)
	f.call(t, "GET", "/api/v1/rooms/"+code, nil, f.member.AccessToken, 200)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	f.call(t, "POST", path+"/playback", map[string]any{"action": "play", "source_version": record.Room.Playback.SourceVersion}, f.root.AccessToken, 200)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	if !record.Room.Playback.Playing {
		t.Fatal("admin playback did not persist")
	}
	oldRoom := record.Room
	f.call(t, "PUT", path, AdminRoomChange{Action: "delete", Version: record.Features.Version, Reason: "test", Confirm: "DELETE ROOM"}, f.root.AccessToken, 200)
	record, _ = repo.AdminReadRoom(context.Background(), code)
	f.call(t, "PUT", path, AdminRoomChange{Action: "restore", Version: record.Features.Version}, f.root.AccessToken, 200)
	f.server.store.UpsertAuthoritativeRoom(oldRoom, 999999)
	current, _ := f.server.store.Room(code)
	if current.AdminEpoch <= oldRoom.AdminEpoch || current.Closed {
		t.Fatal("stale snapshot overwrote administrative state")
	}
	policy := defaultSitePolicy()
	policy.RegistrationEnabled = false
	policy.GuestsEnabled = false
	policy.MaxRoomsPerUser = 1
	policy.MaxRoomMembers = 10
	f.call(t, "PUT", "/api/v1/admin/site-policy", map[string]any{"version": 0, "policy": policy}, f.root.AccessToken, 200)
	f.call(t, "PUT", "/api/v1/admin/site-policy", map[string]any{"version": 0, "policy": policy}, f.root.AccessToken, 409)
	f.call(t, "POST", "/api/v1/auth/register", map[string]any{"email": "closed@management.test", "display_name": "Closed Signup", "password": "correct horse battery"}, "", 403)
	f.call(t, "POST", "/api/v1/rooms", map[string]any{"name": "Second Room", "source_url": "https://example.com/two.mp4"}, f.member.AccessToken, 403)
	f.call(t, "POST", "/api/v1/rooms/"+code+"/guest", map[string]string{"display_name": "Visitor"}, "", 403)
	entries := []TaxonomyEntry{{Key: "films", Name: "电影", Kind: "category", Enabled: true}, {Key: "weekend", Name: "周末", Kind: "tag", Category: "films", Enabled: true}}
	f.call(t, "PUT", "/api/v1/admin/taxonomy", map[string]any{"version": 0, "entries": entries}, f.root.AccessToken, 200)
	raw := f.call(t, "GET", "/api/v1/room-taxonomy", nil, "", 200)
	if !strings.Contains(string(raw), "weekend") {
		t.Fatal("taxonomy not published")
	}
	entries[0].Enabled = false
	f.call(t, "PUT", "/api/v1/admin/taxonomy", map[string]any{"version": 1, "entries": entries}, f.root.AccessToken, 200)
	raw = f.call(t, "GET", "/api/v1/room-taxonomy", nil, "", 200)
	if strings.Contains(string(raw), "weekend") {
		t.Fatal("tag from disabled category is public")
	}
}
func TestAdminSourceDisablingBlocksExistingSourceAndProbeRedactsErrors(t *testing.T) {
	f := newManagementFixture(t, NewMemoryRepository())
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "secret-upstream-password")
	}))
	defer upstream.Close()
	source, err := f.server.sources.CreateWebDAV(ctx, f.member.User, "Test DAV", upstream.URL, "test", "secret-upstream-password")
	if err != nil {
		t.Fatal(err)
	}
	raw := f.call(t, "GET", "/api/v1/admin/media-sources", nil, f.root.AccessToken, 200)
	if strings.Contains(string(raw), "credentials") || strings.Contains(string(raw), "secret-upstream") {
		t.Fatal("source credentials leaked")
	}
	raw = f.call(t, "POST", "/api/v1/admin/media-sources/"+source.ID+"/probe", map[string]string{}, f.root.AccessToken, 200)
	if strings.Contains(string(raw), "secret-upstream") || !strings.Contains(string(raw), `"success":false`) {
		t.Fatalf("unsafe probe response=%s", raw)
	}
	f.call(t, "PUT", "/api/v1/admin/media-sources/"+source.ID, map[string]any{"action": "disable", "version": 1}, f.root.AccessToken, 200)
	if _, err = f.server.sources.PrepareMediaTicket(ctx, f.member.User.ID, source.ID, "/movie.mp4"); err == nil {
		t.Fatal("disabled source still issued media")
	}
	f.call(t, "PUT", "/api/v1/admin/media-sources/"+source.ID, map[string]any{"action": "enable", "version": 1}, f.root.AccessToken, 409)
	f.call(t, "PUT", "/api/v1/admin/media-sources/"+source.ID, map[string]any{"action": "enable", "version": 2}, f.root.AccessToken, 200)
	if _, err = f.server.sources.PrepareMediaTicket(ctx, f.member.User.ID, source.ID, "/movie.mp4"); err != nil {
		t.Fatal(err)
	}
}
func TestPostgresAdminManagementPersistence(t *testing.T) {
	url := testDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	repo, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		run  func(*testing.T, Repository)
	}{{"accounts", testAdminManagementAccounts}, {"rooms", testAdminManagementRooms}} {
		t.Run(item.name, func(t *testing.T) {
			if _, err = repo.pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
				t.Fatal(err)
			}
			item.run(t, repo)
		})
	}
}
