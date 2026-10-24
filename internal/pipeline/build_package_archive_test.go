package pipeline

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestReadSignedBuildPackageArchive(t *testing.T) {
	manifest, ir, wasmBytes := validBuildPackageJSON(t)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := SignBuildPackage(manifest, ir, wasmBytes, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signatureJSON, err := MarshalBuildPackageSignature(signature)
	if err != nil {
		t.Fatal(err)
	}
	archive := buildPackageZIP(t, map[string][]byte{
		"signature.json": signatureJSON,
		"manifest.json":  manifest,
		"module.ir.json": ir,
		"module.wasm":    wasmBytes,
	})
	packageArchive, err := ReadSignedBuildPackageArchive(archive)
	if err != nil {
		t.Fatalf("ReadSignedBuildPackageArchive() returned error: %v", err)
	}
	if !bytes.Equal(packageArchive.SignatureJSON, signatureJSON) ||
		!bytes.Equal(packageArchive.ManifestJSON, manifest) ||
		!bytes.Equal(packageArchive.IRJSON, ir) ||
		!bytes.Equal(packageArchive.WASMBytes, wasmBytes) {
		t.Fatal("parsed build package components differ from the signed transport")
	}
	validatedManifest, err := ValidateSignedBuildPackage(
		packageArchive.SignatureJSON, packageArchive.ManifestJSON,
		packageArchive.IRJSON, packageArchive.WASMBytes,
		map[string]ed25519.PublicKey{signature.KeyID: privateKey.Public().(ed25519.PublicKey)},
	)
	if err != nil {
		t.Fatalf("ValidateSignedBuildPackage() rejected parsed archive: %v", err)
	}
	wasmDigest := sha256.Sum256(packageArchive.WASMBytes)
	if validatedManifest.WASMSHA256 != hex.EncodeToString(wasmDigest[:]) {
		t.Fatalf("validated package Wasm digest = %q, does not match transported artifact", validatedManifest.WASMSHA256)
	}
}

func TestReadSignedBuildPackageArchiveRejectsInvalidEntries(t *testing.T) {
	manifest, ir, wasmBytes := validBuildPackageJSON(t)
	validEntries := map[string][]byte{
		"signature.json": []byte(`{"version":"euromni.build-package-signature.v1","key_id":"` +
			"0000000000000000000000000000000000000000000000000000000000000000" +
			`","signature":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="}`),
		"manifest.json":  manifest,
		"module.ir.json": ir,
		"module.wasm":    wasmBytes,
	}
	tests := []struct {
		name    string
		entries []zipTestEntry
	}{
		{name: "missing entry", entries: zipEntriesWithout(validEntries, "signature.json")},
		{name: "unexpected entry", entries: append(zipEntries(validEntries), zipTestEntry{name: "extra", data: []byte("x")})},
		{name: "duplicate entry", entries: append(zipEntries(validEntries), zipTestEntry{name: "module.wasm", data: wasmBytes})},
		{name: "path entry", entries: append(zipEntriesWithout(validEntries, "manifest.json"), zipTestEntry{name: "../manifest.json", data: manifest})},
		{name: "directory entry", entries: append(zipEntriesWithout(validEntries, "manifest.json"), zipTestEntry{name: "manifest.json/", mode: os.ModeDir})},
		{name: "symlink entry", entries: append(zipEntriesWithout(validEntries, "manifest.json"), zipTestEntry{name: "manifest.json", data: manifest, mode: os.ModeSymlink})},
		{name: "oversized manifest", entries: append(zipEntriesWithout(validEntries, "manifest.json"), zipTestEntry{name: "manifest.json", data: bytes.Repeat([]byte("x"), maxBuildPackageManifestSize+1)})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := buildPackageZIPEntries(t, test.entries)
			if _, err := ReadSignedBuildPackageArchive(archive); err == nil {
				t.Fatal("ReadSignedBuildPackageArchive() accepted invalid entries")
			}
		})
	}
	t.Run("invalid ZIP", func(t *testing.T) {
		if _, err := ReadSignedBuildPackageArchive([]byte("not zip")); err == nil {
			t.Fatal("ReadSignedBuildPackageArchive() accepted invalid ZIP")
		}
	})
	t.Run("oversized transfer", func(t *testing.T) {
		if _, err := ReadSignedBuildPackageArchive(make([]byte, MaxBuildPackageArchiveBytes+1)); err == nil {
			t.Fatal("ReadSignedBuildPackageArchive() accepted oversized transfer")
		}
	})
}

type zipTestEntry struct {
	name   string
	data   []byte
	method uint16
	mode   os.FileMode
}

func buildPackageZIP(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	return buildPackageZIPEntries(t, zipEntries(entries))
}

func buildPackageZIPEntries(t *testing.T, entries []zipTestEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		if entry.method != 0 {
			header.Method = entry.method
		}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if !entry.mode.IsDir() {
			if _, err := file.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func zipEntries(entries map[string][]byte) []zipTestEntry {
	result := make([]zipTestEntry, 0, len(entries))
	for name, data := range entries {
		result = append(result, zipTestEntry{name: name, data: data})
	}
	return result
}

func zipEntriesWithout(entries map[string][]byte, name string) []zipTestEntry {
	filtered := make(map[string][]byte, len(entries)-1)
	for entryName, data := range entries {
		if entryName != name {
			filtered[entryName] = data
		}
	}
	return zipEntries(filtered)
}
