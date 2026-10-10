package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminConfigRBACAnnouncementsMaintenanceAndBotSecrets(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	tokens, _ := NewTokenManager("admin-config-test-secret-with-32-characters", "admin-config")
	auth := NewAuthService(repo, tokens)
	adminDevice := DeviceInfo{ID: "admin-config-super-device", Label: "Admin", Platform: "test"}
	operatorDevice := DeviceInfo{ID: "admin-config-operator-device", Label: "Operator", Platform: "test"}
	admin, _ := auth.Register(ctx, "super@admin.test", "Super", "correct horse battery", adminDevice)
	operator, _ := auth.Register(ctx, "operator@admin.test", "Operator", "another strong password", operatorDevice)
	_ = repo.SetUserAdmin(ctx, admin.User.ID, true)
	_ = repo.VerifyEmail(ctx, admin.User.ID)
	_ = repo.VerifyEmail(ctx, operator.User.ID)
	_ = repo.SetAdminRole(ctx, operator.User.ID, "operator", admin.User.ID)
	admin, _ = auth.Login(ctx, "super@admin.test", "correct horse battery", adminDevice)
	operator, _ = auth.Login(ctx, "operator@admin.test", "another strong password", operatorDevice)
	vaultKey := make([]byte, 32)
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(vaultKey))
	server := NewServer(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"*"}, Repository: repo, Auth: auth, Vault: vault})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	adminPage, err := http.Get(httpServer.URL + "/admin/")
	if err != nil {
		t.Fatal(err)
	}
	adminPage.Body.Close()
	if adminPage.StatusCode != http.StatusOK || adminPage.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("admin web status=%d csp=%q", adminPage.StatusCode, adminPage.Header.Get("Content-Security-Policy"))
	}

	if got := requestStatusDevice(t, http.MethodGet, httpServer.URL+"/api/v1/admin/config", operator.AccessToken, nil, operatorDevice); got != http.StatusForbidden {
		t.Fatalf("operator config status=%d", got)
	}
	config := RuntimeConfig{Maintenance: MaintenanceConfig{Enabled: true, Message: "升级中"}, Features: map[string]bool{"voice": false}, Branding: BrandingConfig{GlobalAnnouncement: "公告"}}
	request := requestDevice(t, http.MethodPut, httpServer.URL+"/api/v1/admin/config", admin.AccessToken, config, adminDevice)
	request.Body.Close()
	if request.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("unconfirmed config status=%d", request.StatusCode)
	}
	response := requestWithHeaders(t, http.MethodPut, httpServer.URL+"/api/v1/admin/config", admin.AccessToken, config, adminDevice, map[string]string{"X-Confirm-Dangerous": "update-runtime-config"})
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("config status=%d", response.StatusCode)
	}
	if got := requestStatusDevice(t, http.MethodGet, httpServer.URL+"/api/v1/clock", operator.AccessToken, nil, operatorDevice); got != http.StatusServiceUnavailable {
		t.Fatalf("maintenance status=%d", got)
	}
	if got := requestStatusDevice(t, http.MethodGet, httpServer.URL+"/api/v1/admin/dashboard", operator.AccessToken, nil, operatorDevice); got != http.StatusOK {
		t.Fatalf("operator dashboard status=%d", got)
	}

	announcementBody := requestJSONDevice(t, http.MethodPost, httpServer.URL+"/api/v1/admin/announcements", operator.AccessToken, map[string]any{"title": "通知", "body": "维护公告", "kind": "startup", "active": true}, operatorDevice)
	var announcement struct {
		Data Announcement `json:"data"`
	}
	_ = json.Unmarshal(announcementBody, &announcement)
	publicBody := requestJSON(t, http.MethodGet, httpServer.URL+"/api/v1/announcements", "", nil)
	var public struct {
		Data struct {
			Announcements []Announcement `json:"announcements"`
		} `json:"data"`
	}
	_ = json.Unmarshal(publicBody, &public)
	if len(public.Data.Announcements) != 1 || public.Data.Announcements[0].ID != announcement.Data.ID {
		t.Fatalf("public announcements=%+v", public)
	}

	bot := map[string]any{"enabled": true, "display_name": "Helper", "summon_policy": "vip", "reply_policy": "mention", "provider_base_url": "https://ai.example.test/v1", "model": "assistant", "credential": "top-secret-key"}
	requestJSONDevice(t, http.MethodPut, httpServer.URL+"/api/v1/admin/room-bot", admin.AccessToken, bot, adminDevice)
	botBody := requestJSONDevice(t, http.MethodGet, httpServer.URL+"/api/v1/admin/room-bot", admin.AccessToken, nil, adminDevice)
	if string(botBody) == "" || containsBytes(botBody, []byte("top-secret-key")) {
		t.Fatalf("bot response leaked credential: %s", botBody)
	}
	_, cipher, _ := repo.GetRoomBotConfig(ctx)
	if cipher == "" || cipher == "top-secret-key" {
		t.Fatal("bot credential was not encrypted")
	}
}

func TestPostgresAdminConfigPersistenceAndDeviceUnban(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repo, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(ctx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatal(err)
	}
	tokens, _ := NewTokenManager("postgres-admin-config-secret-32-characters", "postgres-admin-config")
	auth := NewAuthService(repo, tokens)
	adminDevice := DeviceInfo{ID: "postgres-admin-config-device", Label: "Admin", Platform: "test"}
	memberDevice := DeviceInfo{ID: "postgres-admin-member-device", Label: "Member", Platform: "test"}
	admin, err := auth.Register(ctx, "config-admin@example.test", "Admin", "correct horse battery", adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	member, err := auth.Register(ctx, "config-member@example.test", "Member", "another strong password", memberDevice)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetUserAdmin(ctx, admin.User.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetAdminRole(ctx, member.User.ID, "operator", admin.User.ID); err != nil {
		t.Fatal(err)
	}
	storedMember, err := repo.UserByID(ctx, member.User.ID)
	if err != nil || storedMember.AdminRole != "operator" {
		t.Fatalf("member=%+v error=%v", storedMember, err)
	}

	config := RuntimeConfig{Maintenance: MaintenanceConfig{Enabled: true, Message: "database maintenance"}, Features: map[string]bool{"voice": false}, Branding: BrandingConfig{GlobalAnnouncement: "hello"}}
	config, err = repo.UpdateRuntimeConfig(ctx, config, admin.User.ID)
	if err != nil || !config.Maintenance.Enabled || config.Branding.GlobalAnnouncement != "hello" {
		t.Fatalf("config=%+v error=%v", config, err)
	}
	announcement, err := repo.CreateAnnouncement(ctx, Announcement{Title: "Persistent", Body: "Announcement", Kind: "list", Active: true, CreatedBy: admin.User.ID})
	if err != nil || announcement.ID == 0 {
		t.Fatalf("announcement=%+v error=%v", announcement, err)
	}
	items, err := repo.ListAnnouncements(ctx, true, 0, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("announcements=%+v error=%v", items, err)
	}

	hash := hashDeviceID(memberDevice.ID)
	if err := repo.BanDevice(ctx, hash, "risk", admin.User.ID); err != nil {
		t.Fatal(err)
	}
	bans, err := repo.ListDeviceBans(ctx, 0, 10)
	if err != nil || len(bans) != 1 || bans[0].BannedBy != admin.User.ID {
		t.Fatalf("bans=%+v error=%v", bans, err)
	}
	if err := repo.UnbanDevice(ctx, hash, admin.User.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateDevice(ctx, member.User.ID, hash); err != nil {
		t.Fatalf("unbanned device invalid: %v", err)
	}

	bot, err := repo.UpdateRoomBotConfig(ctx, RoomBotConfig{Enabled: true, DisplayName: "Bot", SummonPolicy: "admin", ReplyPolicy: "mention", ProviderBaseURL: "https://ai.example.test", Model: "test"}, "encrypted-envelope", admin.User.ID)
	if err != nil || !bot.HasCredential {
		t.Fatalf("bot=%+v error=%v", bot, err)
	}
	bot, cipher, err := repo.GetRoomBotConfig(ctx)
	if err != nil || cipher != "encrypted-envelope" || !bot.HasCredential {
		t.Fatalf("stored bot=%+v cipher=%q error=%v", bot, cipher, err)
	}
	dashboard, err := repo.AdminDashboard(ctx)
	if err != nil || dashboard.Users != 2 {
		t.Fatalf("dashboard=%+v error=%v", dashboard, err)
	}
}

func TestAdminDeviceUnbanRestoresOnlyBanRevocations(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	account := AccountRecord{User: User{ID: "member", Email: "member@test", DisplayName: "Member"}, CreatedAt: time.Now()}
	if err := repo.CreateUser(ctx, account); err != nil {
		t.Fatal(err)
	}
	device := DeviceInfo{ID: "member-device", Label: "Member", Platform: "test"}
	hash := hashDeviceID(device.ID)
	if err := repo.BindDevice(ctx, account.ID, hash, device); err != nil {
		t.Fatal(err)
	}
	if err := repo.BanDevice(ctx, hash, "risk", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnbanDevice(ctx, hash, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateDevice(ctx, account.ID, hash); err != nil {
		t.Fatalf("ban revocation not restored: %v", err)
	}
	if err := repo.RevokeUserDevice(ctx, account.ID, hash); err != nil {
		t.Fatal(err)
	}
	if err := repo.BanDevice(ctx, hash, "risk", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnbanDevice(ctx, hash, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateDevice(ctx, account.ID, hash); err != ErrDeviceBlocked {
		t.Fatalf("manual revocation restored: %v", err)
	}
}

func requestWithHeaders(t *testing.T, method, target, token string, payload any, device DeviceInfo, headers map[string]string) *http.Response {
	t.Helper()
	encoded, _ := json.Marshal(payload)
	request, _ := http.NewRequest(method, target, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Device-ID", device.ID)
	request.Header.Set("X-Device-Name", device.Label)
	request.Header.Set("X-Device-Platform", device.Platform)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func containsBytes(value, part []byte) bool {
	return bytes.Contains(value, part)
}
