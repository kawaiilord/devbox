package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	version := flag.String("version", "", "semantic version")
	platform := flag.String("platform", "", "windows, linux, macos, android, ios, or web")
	packageURL := flag.String("url", "", "HTTPS package URL")
	packagePath := flag.String("package", "", "package path")
	output := flag.String("output", "manifest.json", "manifest output path")
	published := flag.String("published-at", time.Now().UTC().Format(time.RFC3339), "RFC3339 publication time")
	flag.Parse()
	if *version == "" || *platform == "" || *packageURL == "" || *packagePath == "" {
		fatal("version, platform, url, and package are required")
	}
	privateRaw, err := base64.StdEncoding.DecodeString(os.Getenv("SAMEFRAME_UPDATE_PRIVATE_KEY"))
	if err != nil || len(privateRaw) != ed25519.PrivateKeySize {
		fatal("SAMEFRAME_UPDATE_PRIVATE_KEY must be a base64 Ed25519 private key")
	}
	content, err := os.ReadFile(*packagePath)
	if err != nil {
		fatal(err.Error())
	}
	hash := sha256.Sum256(content)
	hashText := hex.EncodeToString(hash[:])
	payload := fmt.Sprintf("%s\n%s\n%s\n%s\n%d\n%s", *version, *platform, *packageURL, hashText, len(content), *published)
	signature := ed25519.Sign(ed25519.PrivateKey(privateRaw), []byte(payload))
	manifest := map[string]any{"version": *version, "platform": *platform, "url": *packageURL, "sha256": hashText, "size": len(content), "published_at": *published, "signature": base64.StdEncoding.EncodeToString(signature)}
	encoded, _ := json.MarshalIndent(manifest, "", "  ")
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*output, encoded, 0o644); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
