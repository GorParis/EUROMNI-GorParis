package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateBuildManifestJSON(t *testing.T) {
	wasm := []byte("test wasm")
	digest := sha256.Sum256(wasm)
	valid, err := json.Marshal(struct {
		WASMSHA256 string `json:"wasm_sha256"`
	}{WASMSHA256: hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBuildManifestJSON(digest[:], valid); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	tests := []struct {
		name string
		hash []byte
		json []byte
	}{
		{name: "short digest", hash: digest[:3], json: valid},
		{name: "malformed JSON", hash: digest[:], json: []byte(`{"wasm_sha256":`)},
		{name: "trailing JSON", hash: digest[:], json: append(append([]byte(nil), valid...), []byte(` {}`)...)},
		{name: "wrong Wasm hash", hash: make([]byte, sha256.Size), json: valid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBuildManifestJSON(test.hash, test.json); !errors.Is(err, ErrInvalidBuildManifest) {
				t.Fatalf("validation error = %v, want ErrInvalidBuildManifest", err)
			}
		})
	}
}
