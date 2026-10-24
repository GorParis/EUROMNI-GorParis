package pipeline

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTrustedBuildPackageKeys(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	document := trustedBuildKeysDocument{
		Version: trustedBuildKeysVersion,
		Keys: []trustedBuildPackageKey{{
			KeyID:     buildPackageKeyID(publicKey),
			PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		}},
	}
	path := writeTrustedBuildKeysFile(t, document)

	keys, err := LoadTrustedBuildPackageKeys(path)
	if err != nil {
		t.Fatalf("LoadTrustedBuildPackageKeys() returned error: %v", err)
	}
	loaded, exists := keys[buildPackageKeyID(publicKey)]
	if !exists || string(loaded) != string(publicKey) {
		t.Fatal("trusted build key was not loaded")
	}
	loaded[0] ^= 0xff
	if string(publicKey) == string(loaded) {
		t.Fatal("loaded key unexpectedly shares the caller's public key slice")
	}
}

func TestLoadTrustedBuildPackageKeysRejectsInvalidDocuments(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	validEntry := trustedBuildPackageKey{
		KeyID:     buildPackageKeyID(publicKey),
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}
	tests := []struct {
		name     string
		document string
	}{
		{
			name:     "unsupported version",
			document: `{"version":"euromni.trusted-build-keys.v2","keys":[]}`,
		},
		{
			name:     "missing keys array",
			document: `{"version":"euromni.trusted-build-keys.v1"}`,
		},
		{
			name:     "null keys array",
			document: `{"version":"euromni.trusted-build-keys.v1","keys":null}`,
		},
		{
			name:     "unknown field",
			document: `{"version":"euromni.trusted-build-keys.v1","keys":[],"extra":true}`,
		},
		{
			name:     "trailing document",
			document: `{"version":"euromni.trusted-build-keys.v1","keys":[]} {}`,
		},
		{
			name:     "invalid base64",
			document: marshalTrustedBuildKeysDocument(t, trustedBuildKeysDocument{Version: trustedBuildKeysVersion, Keys: []trustedBuildPackageKey{{KeyID: validEntry.KeyID, PublicKey: "not-base64"}}}),
		},
		{
			name:     "mismatched key ID",
			document: marshalTrustedBuildKeysDocument(t, trustedBuildKeysDocument{Version: trustedBuildKeysVersion, Keys: []trustedBuildPackageKey{{KeyID: strings.Repeat("0", 64), PublicKey: validEntry.PublicKey}}}),
		},
		{
			name: "duplicate key ID",
			document: marshalTrustedBuildKeysDocument(t, trustedBuildKeysDocument{
				Version: trustedBuildKeysVersion,
				Keys:    []trustedBuildPackageKey{validEntry, validEntry},
			}),
		},
		{
			name: "invalid public key length",
			document: marshalTrustedBuildKeysDocument(t, trustedBuildKeysDocument{
				Version: trustedBuildKeysVersion,
				Keys: []trustedBuildPackageKey{{
					KeyID:     validEntry.KeyID,
					PublicKey: base64.StdEncoding.EncodeToString([]byte("short")),
				}},
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTrustedBuildKeysBytes(t, []byte(test.document))
			if _, err := LoadTrustedBuildPackageKeys(path); err == nil {
				t.Fatal("LoadTrustedBuildPackageKeys() accepted an invalid document")
			}
		})
	}

	t.Run("oversized file", func(t *testing.T) {
		path := writeTrustedBuildKeysBytes(t, make([]byte, maxTrustedBuildKeysBytes+1))
		if _, err := LoadTrustedBuildPackageKeys(path); err == nil {
			t.Fatal("LoadTrustedBuildPackageKeys() accepted an oversized file")
		}
	})
	t.Run("empty path", func(t *testing.T) {
		if _, err := LoadTrustedBuildPackageKeys(" "); err == nil {
			t.Fatal("LoadTrustedBuildPackageKeys() accepted an empty path")
		}
	})
	t.Run("too many keys", func(t *testing.T) {
		document := trustedBuildKeysDocument{
			Version: trustedBuildKeysVersion,
			Keys:    make([]trustedBuildPackageKey, maxTrustedBuildKeyCount+1),
		}
		if _, err := LoadTrustedBuildPackageKeys(writeTrustedBuildKeysFile(t, document)); err == nil {
			t.Fatal("LoadTrustedBuildPackageKeys() accepted too many keys")
		}
	})
	t.Run("empty trust set", func(t *testing.T) {
		document := trustedBuildKeysDocument{
			Version: trustedBuildKeysVersion,
			Keys:    []trustedBuildPackageKey{},
		}
		keys, err := LoadTrustedBuildPackageKeys(writeTrustedBuildKeysFile(t, document))
		if err != nil {
			t.Fatalf("LoadTrustedBuildPackageKeys() rejected an empty trust set: %v", err)
		}
		if len(keys) != 0 {
			t.Fatalf("trusted key count = %d, want 0", len(keys))
		}
	})
}

func writeTrustedBuildKeysFile(t *testing.T, document trustedBuildKeysDocument) string {
	t.Helper()
	return writeTrustedBuildKeysBytes(t, []byte(marshalTrustedBuildKeysDocument(t, document)))
}

func writeTrustedBuildKeysBytes(t *testing.T, contents []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trusted-keys.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func marshalTrustedBuildKeysDocument(t *testing.T, document trustedBuildKeysDocument) string {
	t.Helper()
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
