package pipeline

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestSignAndValidateSignedBuildPackage(t *testing.T) {
	manifestJSON, irJSON, wasmBytes := validBuildPackageJSON(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
	if err != nil {
		t.Fatalf("SignBuildPackage() returned error: %v", err)
	}
	if envelope.KeyID != buildPackageKeyID(publicKey) {
		t.Fatalf("signature key ID = %q, want %q", envelope.KeyID, buildPackageKeyID(publicKey))
	}
	signatureJSON, err := MarshalBuildPackageSignature(envelope)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ValidateSignedBuildPackage(signatureJSON, manifestJSON, irJSON, wasmBytes,
		map[string]ed25519.PublicKey{envelope.KeyID: publicKey})
	if err != nil {
		t.Fatalf("ValidateSignedBuildPackage() rejected a signed package: %v", err)
	}
	if manifest.WASMSHA256 == "" {
		t.Fatal("validated manifest is missing its Wasm digest")
	}
}

func TestValidateSignedBuildPackageRejectsUntrustedOrChangedInputs(t *testing.T) {
	manifestJSON, irJSON, wasmBytes := validBuildPackageJSON(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signatureJSON, err := MarshalBuildPackageSignature(envelope)
	if err != nil {
		t.Fatal(err)
	}
	trustedKeys := map[string]ed25519.PublicKey{envelope.KeyID: publicKey}

	modifiedWasm := append([]byte(nil), wasmBytes...)
	modifiedWasm[len(modifiedWasm)-1] ^= 1
	tests := []struct {
		name        string
		signature   []byte
		manifest    []byte
		ir          []byte
		wasm        []byte
		trustedKeys map[string]ed25519.PublicKey
	}{
		{
			name:      "modified artifact",
			signature: signatureJSON, manifest: manifestJSON, ir: irJSON,
			wasm: modifiedWasm, trustedKeys: trustedKeys,
		},
		{
			name:      "untrusted signer",
			signature: signatureJSON, manifest: manifestJSON, ir: irJSON,
			wasm: wasmBytes, trustedKeys: map[string]ed25519.PublicKey{},
		},
		{
			name: "modified signature",
			signature: mustMarshalSignature(t, BuildPackageSignature{
				Version: envelope.Version, KeyID: envelope.KeyID,
				Signature: base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)),
			}),
			manifest: manifestJSON, ir: irJSON, wasm: wasmBytes, trustedKeys: trustedKeys,
		},
		{
			name:      "unknown envelope field",
			signature: append(signatureJSON[:len(signatureJSON)-1], []byte(`,"extra":true}`)...),
			manifest:  manifestJSON, ir: irJSON, wasm: wasmBytes, trustedKeys: trustedKeys,
		},
		{
			name:      "noncanonical envelope",
			signature: append([]byte(" "), signatureJSON...),
			manifest:  manifestJSON, ir: irJSON, wasm: wasmBytes, trustedKeys: trustedKeys,
		},
		{
			name:      "oversized envelope",
			signature: make([]byte, maxBuildPackageSignatureEnvelopeBytes+1),
			manifest:  manifestJSON, ir: irJSON, wasm: wasmBytes, trustedKeys: trustedKeys,
		},
		{
			name: "noncanonical signature encoding",
			signature: mustMarshalSignature(t, BuildPackageSignature{
				Version: envelope.Version, KeyID: envelope.KeyID, Signature: "not-base64",
			}),
			manifest: manifestJSON, ir: irJSON, wasm: wasmBytes, trustedKeys: trustedKeys,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ValidateSignedBuildPackage(test.signature, test.manifest, test.ir, test.wasm, test.trustedKeys); err == nil {
				t.Fatal("ValidateSignedBuildPackage() accepted invalid package signature")
			}
		})
	}
}

func TestSignBuildPackageRejectsInvalidPackageAndPrivateKey(t *testing.T) {
	manifestJSON, irJSON, wasmBytes := validBuildPackageJSON(t)
	if _, err := SignBuildPackage(manifestJSON, irJSON, wasmBytes, ed25519.PrivateKey{}); err == nil {
		t.Fatal("SignBuildPackage() accepted an invalid private key")
	}
	inconsistentKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	inconsistentKey[len(inconsistentKey)-1] ^= 1
	if _, err := SignBuildPackage(manifestJSON, irJSON, wasmBytes, inconsistentKey); err == nil {
		t.Fatal("SignBuildPackage() accepted an inconsistent private key")
	}
	if _, err := SignBuildPackage(manifestJSON, irJSON, wasmBytes[:len(wasmBytes)-1], ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))); err == nil {
		t.Fatal("SignBuildPackage() signed an invalid package")
	}
}

func TestBuildPackageSigningMessageUsesVersionedRawDigests(t *testing.T) {
	manifestJSON, irJSON, wasmBytes := validBuildPackageJSON(t)
	message := buildPackageSigningMessage(manifestJSON, irJSON, wasmBytes)
	expected := append([]byte(buildPackageSignatureVersion+"\x00"), rawDigest(manifestJSON)...)
	expected = append(expected, rawDigest(irJSON)...)
	expected = append(expected, rawDigest(wasmBytes)...)
	if !bytes.Equal(message, expected) {
		t.Fatal("signing message does not contain the domain separator and raw component digests")
	}
	if len(message) != len(buildPackageSignatureVersion)+1+3*sha256.Size {
		t.Fatalf("signing message size = %d, want %d", len(message), len(buildPackageSignatureVersion)+1+3*sha256.Size)
	}
}

func rawDigest(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}

func validBuildPackageJSON(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	ir := testCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error: %v", err)
	}
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatalf("NewBuildManifest() returned error: %v", err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	irJSON, err := json.Marshal(ir)
	if err != nil {
		t.Fatalf("marshal IR: %v", err)
	}
	return manifestJSON, irJSON, wasmBytes
}

func mustMarshalSignature(t *testing.T, signature BuildPackageSignature) []byte {
	t.Helper()
	encoded, err := json.Marshal(signature)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
