package app

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"strings"
)

type CredentialVault struct {
	aead cipher.AEAD
}

func NewCredentialVault(encodedKey string) (*CredentialVault, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, errors.New("vault key must be a base64-encoded 32-byte value")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	return &CredentialVault{aead: aead}, nil
}

func (v *CredentialVault) Encrypt(plaintext []byte, associatedData string) (string, error) {
	sealed := v.aead.Seal(nil, nil, plaintext, []byte(associatedData))
	return "v1." + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (v *CredentialVault) Decrypt(envelope, associatedData string) ([]byte, error) {
	version, encoded, ok := strings.Cut(envelope, ".")
	if !ok || version != "v1" {
		return nil, errors.New("unsupported credential envelope")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid credential envelope")
	}
	plaintext, err := v.aead.Open(nil, nil, sealed, []byte(associatedData))
	if err != nil {
		return nil, errors.New("credential authentication failed")
	}
	return plaintext, nil
}
