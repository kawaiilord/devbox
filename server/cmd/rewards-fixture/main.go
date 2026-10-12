// rewards-fixture runs an isolated, in-memory browser acceptance environment.
// It binds only loopback and is never used by the production server command.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/kawaiilord/devbox/server/internal/app"
)

func main() {
	verified := flag.Bool("verified-access", false, "require accounts and verified registration in this isolated fixture")
	credentials := flag.String("credentials", "", "private credentials output file (required)")
	flag.Parse()
	if *credentials == "" {
		log.Fatal("--credentials is required")
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		log.Fatal(err)
	}
	secret := hex.EncodeToString(token)
	password := "Fixture-" + secret[:24]
	ctx := context.Background()
	repo := app.NewMemoryRepository()
	tokens, err := app.NewTokenManager(secret, "rewards-fixture")
	if err != nil {
		log.Fatal(err)
	}
	auth := app.NewAuthService(repo, tokens, fixtureMailer{path: *credentials + ".mail.json"})
	device := app.DeviceInfo{ID: "rewards-fixture-user-device", Label: "Rewards fixture", Platform: "test"}
	session, err := auth.Register(ctx, "viewer@rewards.example.test", "同帧体验官", password, device)
	if err != nil {
		log.Fatal(err)
	}
	for i := 1; i <= 29; i++ {
		if _, err = repo.DailyCheckIn(ctx, session.User.ID, time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02"), 1); err != nil {
			log.Fatal(err)
		}
	}
	peer, err := auth.Register(ctx, "peer@rewards.example.test", "铂金观影搭子", password, app.DeviceInfo{ID: "rewards-fixture-peer-device", Label: "Peer fixture", Platform: "test"})
	if err != nil {
		log.Fatal(err)
	}
	if err = repo.SetUserVIP(ctx, peer.User.ID, time.Now().AddDate(0, 0, 30).UnixMilli()); err != nil {
		log.Fatal(err)
	}
	if err = repo.FollowUser(ctx, session.User.ID, peer.User.ID); err != nil {
		log.Fatal(err)
	}
	file, err := os.OpenFile(*credentials, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	if err = json.NewEncoder(file).Encode(map[string]string{"email": session.User.Email, "password": password}); err != nil {
		log.Fatal(err)
	}
	file.Close()
	server := app.NewServer(app.Options{Repository: repo, Auth: auth, PointsPerCheckIn: 1, RequireAccount: *verified, RequireRegistrationCode: *verified, AllowedOrigins: []string{"http://localhost:18883"}})
	listener, err := net.Listen("tcp", "127.0.0.1:18882")
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Rewards fixture listening on 127.0.0.1:18882; 29 seeded historical points")
	log.Fatal(http.Serve(listener, server.Handler()))
}

// Verification messages are private, local test artifacts, never an HTTP endpoint.
type fixtureMailer struct{ path string }

func (m fixtureMailer) Send(_ context.Context, message app.MailMessage) error {
	if message.Type != "register_code" {
		return nil
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0600)
}
