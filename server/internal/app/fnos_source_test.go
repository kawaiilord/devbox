package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestFnosEncryptedOTPLoginNativeBrowseAndWebDAVPlayback(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustPublicKey(t, &key.PublicKey)}))
	signingSecret := []byte("fnos-test-signing-secret")
	mux := http.NewServeMux()
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if len(data) > 44 && data[0] != '{' {
				mac := hmac.New(sha256.New, signingSecret)
				mac.Write(data[44:])
				if string(data[:44]) != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
					t.Error("FNOS request signature mismatch")
					return
				}
				data = data[44:]
			}
			var request map[string]any
			if json.Unmarshal(data, &request) != nil {
				t.Error("invalid FNOS request")
				return
			}
			var aesKey, iv []byte
			if request["req"] == "encrypted" {
				encryptedKey, _ := base64.StdEncoding.DecodeString(textValue(request["rsa"]))
				aesKey, err = rsa.DecryptPKCS1v15(rand.Reader, key, encryptedKey)
				if err != nil {
					t.Error(err)
					return
				}
				iv, _ = base64.StdEncoding.DecodeString(textValue(request["iv"]))
				payload, _ := base64.StdEncoding.DecodeString(textValue(request["aes"]))
				block, _ := aes.NewCipher(aesKey)
				cipher.NewCBCDecrypter(block, iv).CryptBlocks(payload, payload)
				n := int(payload[len(payload)-1])
				if n < 1 || n > 16 {
					t.Error("bad login padding")
					return
				}
				if json.Unmarshal(payload[:len(payload)-n], &request) != nil {
					t.Error("bad encrypted payload")
					return
				}
			}
			response := map[string]any{"reqid": request["reqid"], "result": "succ"}
			switch request["req"] {
			case "util.crypto.getRSAPub":
				response["pub"] = public
				response["si"] = "fnos-session"
			case "user.login":
				if request["user"] != "viewer" || request["password"] != "top-secret" || request["si"] != "fnos-session" {
					t.Error("FNOS credentials/session missing")
				}
				response["accessToken"] = "otp-challenge"
			case "user.2fa.loginVerify":
				if request["code"] != "123456" || request["accessToken"] != "otp-challenge" {
					t.Error("OTP not verified")
				}
				block, _ := aes.NewCipher(aesKey)
				padded := append([]byte(nil), signingSecret...)
				n := aes.BlockSize - len(padded)%aes.BlockSize
				padded = append(padded, bytes.Repeat([]byte{byte(n)}, n)...)
				encrypted := make([]byte, len(padded))
				cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
				response["token"] = "fnos-token"
				response["longToken"] = "long-fnos-token"
				response["secret"] = base64.StdEncoding.EncodeToString(encrypted)
			case "user.authToken":
				if request["token"] != "fnos-token" || request["si"] != "fnos-session" {
					t.Error("token authentication failed")
				}
			case "file.ls":
				if request["path"] == nil {
					response["files"] = []any{map[string]any{"name": "Movies", "v": 1, "uid": 42, "dir": 1}}
				} else {
					if request["path"] != "vol1/42/Movies" {
						t.Errorf("unexpected FNOS directory %v", request["path"])
					}
					response["files"] = []any{map[string]any{"name": "movie.mp4", "dir": 0, "size": 10}}
				}
			default:
				t.Errorf("unexpected FNOS method %v", request["req"])
				return
			}
			encoded, _ := json.Marshal(response)
			if conn.Write(ctx, websocket.MessageText, encoded) != nil {
				return
			}
		}
	})
	mux.HandleFunc("/dav/Movies/movie.mp4", func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if u != "viewer" || p != "top-secret" || r.Header.Get("Range") != "bytes=0-3" {
			t.Error("FNOS media auth/range incorrect")
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.WriteHeader(206)
		_, _ = io.WriteString(w, "data")
	})
	upstream := httptest.NewServer(mux)
	defer upstream.Close()
	repo := NewMemoryRepository()
	vault, _ := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	manager := NewMediaSourceManager(repo, vault, true)
	source, err := manager.SaveNAS(context.Background(), User{ID: "owner"}, "", NASSourceInput{Provider: "fnos", Name: "Home fnOS", BaseURL: upstream.URL, Username: "viewer", Password: "top-secret", OTP: "123456", WebDAVURL: upstream.URL + "/dav/"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := manager.Browse(context.Background(), "owner", source.ID, "/")
	if err != nil || len(root) != 1 || root[0].Path != "/vol1/42/Movies" {
		t.Fatalf("root=%+v error=%v", root, err)
	}
	files, err := manager.Browse(context.Background(), "owner", source.ID, root[0].Path)
	if err != nil || len(files) != 1 {
		t.Fatal(err)
	}
	ticket, err := manager.PrepareMediaTicket(context.Background(), "owner", source.ID, files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	response, err := manager.Open(context.Background(), ticket, "GET", "bytes=0-3", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "data" {
		t.Fatal("FNOS video stream failed")
	}
	_, secret, _ := manager.loadSourceSecret(context.Background(), "owner", source.ID)
	if strings.Contains(string(secret), "123456") {
		t.Fatal("OTP was persisted")
	}
	sourceJSON, _ := json.Marshal(source)
	if strings.Contains(string(sourceJSON), "fnos-token") || strings.Contains(string(sourceJSON), "top-secret") {
		t.Fatal("FNOS credentials leaked")
	}
}

func mustPublicKey(t *testing.T, key *rsa.PublicKey) []byte {
	t.Helper()
	b, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
