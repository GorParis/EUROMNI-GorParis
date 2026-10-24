package pipeline

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const buildPackageSignatureVersion = "euromni.build-package-signature.v1"
const maxBuildPackageSignatureEnvelopeBytes = 4096

type BuildPackageSignature struct {
	Version   string `json:"version"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

// SignBuildPackage validates the package and signs its exact component digests.
func SignBuildPackage(manifestJSON, irJSON, wasmBytes []byte, privateKey ed25519.PrivateKey) (BuildPackageSignature, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return BuildPackageSignature{}, errors.New("Ed25519 private key has an invalid length")
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(privateKey[:ed25519.SeedSize]), privateKey) {
		return BuildPackageSignature{}, errors.New("Ed25519 private key is internally inconsistent")
	}
	if _, err := ValidateBuildPackage(manifestJSON, irJSON, wasmBytes); err != nil {
		return BuildPackageSignature{}, fmt.Errorf("validate build package before signing: %w", err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signature := ed25519.Sign(privateKey, buildPackageSigningMessage(manifestJSON, irJSON, wasmBytes))
	return BuildPackageSignature{
		Version:   buildPackageSignatureVersion,
		KeyID:     buildPackageKeyID(publicKey),
		Signature: base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// ValidateSignedBuildPackage verifies a trusted signature before structural validation.
func ValidateSignedBuildPackage(signatureJSON, manifestJSON, irJSON, wasmBytes []byte, trustedKeys map[string]ed25519.PublicKey) (BuildManifest, error) {
	if len(signatureJSON) == 0 || len(signatureJSON) > maxBuildPackageSignatureEnvelopeBytes {
		return BuildManifest{}, errors.New("build package signature envelope has an invalid size")
	}
	var envelope BuildPackageSignature
	if err := decodeStrictJSON(signatureJSON, &envelope); err != nil {
		return BuildManifest{}, fmt.Errorf("decode build package signature: %w", err)
	}
	canonicalJSON, err := json.Marshal(envelope)
	if err != nil {
		return BuildManifest{}, fmt.Errorf("encode canonical build package signature: %w", err)
	}
	if string(signatureJSON) != string(canonicalJSON) {
		return BuildManifest{}, errors.New("build package signature is not in canonical JSON form")
	}
	if envelope.Version != buildPackageSignatureVersion {
		return BuildManifest{}, fmt.Errorf("unsupported build package signature version %q", envelope.Version)
	}
	publicKey, trusted := trustedKeys[envelope.KeyID]
	if !trusted {
		return BuildManifest{}, errors.New("build package signer is not trusted")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return BuildManifest{}, errors.New("trusted Ed25519 public key has an invalid length")
	}
	if buildPackageKeyID(publicKey) != envelope.KeyID {
		return BuildManifest{}, errors.New("trusted Ed25519 public key does not match its key ID")
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || base64.StdEncoding.EncodeToString(signature) != envelope.Signature {
		return BuildManifest{}, errors.New("build package signature is not canonical base64")
	}
	if len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(publicKey, buildPackageSigningMessage(manifestJSON, irJSON, wasmBytes), signature) {
		return BuildManifest{}, errors.New("build package signature verification failed")
	}
	manifest, err := ValidateBuildPackage(manifestJSON, irJSON, wasmBytes)
	if err != nil {
		return BuildManifest{}, fmt.Errorf("validate signed build package: %w", err)
	}
	return manifest, nil
}

func buildPackageKeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:])
}

func buildPackageSigningMessage(manifestJSON, irJSON, wasmBytes []byte) []byte {
	message := make([]byte, 0, len(buildPackageSignatureVersion)+1+3*sha256.Size)
	message = append(message, buildPackageSignatureVersion...)
	message = append(message, 0)
	for _, component := range [][]byte{manifestJSON, irJSON, wasmBytes} {
		digest := sha256.Sum256(component)
		message = append(message, digest[:]...)
	}
	return message
}

func MarshalBuildPackageSignature(signature BuildPackageSignature) ([]byte, error) {
	encoded, err := json.Marshal(signature)
	if err != nil {
		return nil, fmt.Errorf("encode build package signature: %w", err)
	}
	return encoded, nil
}
