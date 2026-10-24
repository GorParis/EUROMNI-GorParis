package pipeline

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	trustedBuildKeysVersion  = "euromni.trusted-build-keys.v1"
	maxTrustedBuildKeysBytes = 64 << 10
	maxTrustedBuildKeyCount  = 256
)

type trustedBuildKeysDocument struct {
	Version string                   `json:"version"`
	Keys    []trustedBuildPackageKey `json:"keys"`
}

type trustedBuildPackageKey struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
}

// LoadTrustedBuildPackageKeys reads an explicit per-installation trust file.
func LoadTrustedBuildPackageKeys(path string) (map[string]ed25519.PublicKey, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("trusted build keys file path is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trusted build keys file: %w", err)
	}
	defer file.Close()

	documentBytes, err := io.ReadAll(io.LimitReader(file, maxTrustedBuildKeysBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read trusted build keys file: %w", err)
	}
	if len(documentBytes) > maxTrustedBuildKeysBytes {
		return nil, fmt.Errorf("trusted build keys file exceeds %d bytes", maxTrustedBuildKeysBytes)
	}

	var document trustedBuildKeysDocument
	if err := decodeStrictJSON(documentBytes, &document); err != nil {
		return nil, fmt.Errorf("decode trusted build keys file: %w", err)
	}
	if document.Version != trustedBuildKeysVersion {
		return nil, fmt.Errorf("unsupported trusted build keys version %q", document.Version)
	}
	if document.Keys == nil {
		return nil, errors.New("trusted build keys file must contain a keys array")
	}
	if len(document.Keys) > maxTrustedBuildKeyCount {
		return nil, fmt.Errorf("trusted build keys file contains more than %d keys", maxTrustedBuildKeyCount)
	}

	keys := make(map[string]ed25519.PublicKey, len(document.Keys))
	for index, entry := range document.Keys {
		publicKey, err := base64.StdEncoding.DecodeString(entry.PublicKey)
		if err != nil || base64.StdEncoding.EncodeToString(publicKey) != entry.PublicKey {
			return nil, fmt.Errorf("trusted build key %d has invalid canonical base64", index)
		}
		if len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("trusted build key %d has invalid Ed25519 public key length", index)
		}
		if buildPackageKeyID(ed25519.PublicKey(publicKey)) != entry.KeyID {
			return nil, fmt.Errorf("trusted build key %d ID does not match its public key", index)
		}
		if _, exists := keys[entry.KeyID]; exists {
			return nil, fmt.Errorf("trusted build keys file contains duplicate key ID %q", entry.KeyID)
		}
		keys[entry.KeyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return keys, nil
}
