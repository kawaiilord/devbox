package app

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCredentialVaultAuthenticatesCiphertextAndOwnership(t *testing.T) {
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	vault, err := NewCredentialVault(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := vault.Encrypt([]byte(`{"username":"viewer","password":"secret"}`), "user-a:source-a")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := vault.Decrypt(envelope, "user-a:source-a")
	if err != nil || string(plaintext) != `{"username":"viewer","password":"secret"}` {
		t.Fatalf("plaintext=%q error=%v", plaintext, err)
	}
	if _, err := vault.Decrypt(envelope, "user-b:source-a"); err == nil {
		t.Fatal("credential decrypted with wrong ownership AAD")
	}
	_, encoded, _ := strings.Cut(envelope, ".")
	sealed, _ := base64.RawURLEncoding.DecodeString(encoded)
	sealed[len(sealed)/2] ^= 0x01
	tampered := "v1." + base64.RawURLEncoding.EncodeToString(sealed)
	if _, err := vault.Decrypt(tampered, "user-a:source-a"); err == nil {
		t.Fatal("tampered credential envelope decrypted")
	}
}
