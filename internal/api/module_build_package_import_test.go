package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"euromni/internal/packagecandidate"
	"euromni/internal/pipeline"
	"euromni/internal/seed"
)

func TestAdminImportsSignedBuildPackageAsShopScopedCandidate(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, irJSON, wasmBytes := makeSignedBuildPackageParts(t)
	signature, err := pipeline.SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signatureJSON, err := pipeline.MarshalBuildPackageSignature(signature)
	if err != nil {
		t.Fatal(err)
	}
	archive := makeSignedBuildPackageZIP(t, map[string][]byte{
		"signature.json": signatureJSON,
		"manifest.json":  manifestJSON,
		"module.ir.json": irJSON,
		"module.wasm":    wasmBytes,
	})

	root := filepath.Join(t.TempDir(), "candidates")
	candidates, err := packagecandidate.NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	oldCandidates := moduleBuildPackageCandidates
	oldKeys := TrustedBuildPackageKeys()
	oldAuth := staffAuthenticator
	SetModuleBuildPackageCandidates(candidates)
	SetTrustedBuildPackageKeys(map[string]ed25519.PublicKey{signature.KeyID: publicKey})
	t.Cleanup(func() {
		SetModuleBuildPackageCandidates(oldCandidates)
		SetTrustedBuildPackageKeys(oldKeys)
		SetStaffAuthenticator(oldAuth)
	})

	admin := newCatalogTestAuthenticator("admin", seed.ShopID)
	mux := http.NewServeMux()
	Register(mux)
	editor := newCatalogTestAuthenticator("editor", seed.ShopID)
	SetStaffAuthenticator(editor)
	if response := performSignedPackageImport(mux, editor.token, archive); response.Code != http.StatusForbidden {
		t.Fatalf("editor import status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body)
	}
	SetStaffAuthenticator(admin)
	signedBuildPackageImportSlot <- struct{}{}
	if response := performSignedPackageImport(mux, admin.token, archive); response.Code != http.StatusServiceUnavailable {
		<-signedBuildPackageImportSlot
		t.Fatalf("concurrent import status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body)
	}
	<-signedBuildPackageImportSlot
	response := performSignedPackageImport(mux, admin.token, archive)
	if response.Code != http.StatusAccepted {
		t.Fatalf("signed build package import status = %d, want %d; body=%s", response.Code, http.StatusAccepted, response.Body)
	}
	var receipt struct {
		Data struct {
			PackageSHA256 string `json:"package_sha256"`
			Size          int64  `json:"size"`
			Status        string `json:"status"`
			Module        string `json:"module"`
			SignerKeyID   string `json:"signer_key_id"`
			WASMSHA256    string `json:"wasm_sha256"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("decode build package candidate receipt: %v", err)
	}
	if receipt.Data.PackageSHA256 == "" || receipt.Data.Size != int64(len(archive)) ||
		receipt.Data.Status != "candidate" || receipt.Data.Module != "demo.catalog-reader" ||
		receipt.Data.SignerKeyID != signature.KeyID {
		t.Fatalf("build package candidate receipt = %+v", receipt.Data)
	}
	stored, err := candidates.Get(context.Background(), seed.ShopID, receipt.Data.PackageSHA256)
	if err != nil || !bytes.Equal(stored, archive) {
		t.Fatalf("stored candidate size/error = %d/%v", len(stored), err)
	}
	if _, err := candidates.Get(context.Background(), "other-shop", receipt.Data.PackageSHA256); err == nil {
		t.Fatal("candidate was visible in a different shop scope")
	}
	otherShop := newCatalogTestAuthenticator("admin", "e0e00000-0001-4000-8000-000000000099")
	SetStaffAuthenticator(otherShop)
	otherShopResponse := performSignedPackageImport(mux, otherShop.token, archive)
	if otherShopResponse.Code != http.StatusAccepted {
		t.Fatalf("other shop import status = %d, want %d; body=%s", otherShopResponse.Code, http.StatusAccepted, otherShopResponse.Body)
	}
	if stored, err := candidates.Get(context.Background(), "e0e00000-0001-4000-8000-000000000099", receipt.Data.PackageSHA256); err != nil ||
		!bytes.Equal(stored, archive) {
		t.Fatalf("other shop candidate size/error = %d/%v", len(stored), err)
	}
}

func TestAdminSignedBuildPackageImportRejectsUntrustedSignerWithoutStorage(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, irJSON, wasmBytes := makeSignedBuildPackageParts(t)
	signature, err := pipeline.SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signatureJSON, err := pipeline.MarshalBuildPackageSignature(signature)
	if err != nil {
		t.Fatal(err)
	}
	archive := makeSignedBuildPackageZIP(t, map[string][]byte{
		"signature.json": signatureJSON,
		"manifest.json":  manifestJSON,
		"module.ir.json": irJSON,
		"module.wasm":    wasmBytes,
	})
	candidateRoot := filepath.Join(t.TempDir(), "candidates")
	candidates, err := packagecandidate.NewFileStore(candidateRoot)
	if err != nil {
		t.Fatal(err)
	}
	oldCandidates := moduleBuildPackageCandidates
	oldKeys := TrustedBuildPackageKeys()
	oldAuth := staffAuthenticator
	SetModuleBuildPackageCandidates(candidates)
	SetTrustedBuildPackageKeys(map[string]ed25519.PublicKey{})
	admin := newCatalogTestAuthenticator("admin", seed.ShopID)
	SetStaffAuthenticator(admin)
	t.Cleanup(func() {
		SetModuleBuildPackageCandidates(oldCandidates)
		SetTrustedBuildPackageKeys(oldKeys)
		SetStaffAuthenticator(oldAuth)
	})

	mux := http.NewServeMux()
	Register(mux)
	response := performSignedPackageImport(mux, admin.token, archive)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "rejected") {
		t.Fatalf("untrusted package response = %d/%s, want rejected 400", response.Code, response.Body)
	}
	digest := sha256.Sum256(archive)
	if _, err := candidates.Get(context.Background(), seed.ShopID, hex.EncodeToString(digest[:])); err == nil {
		t.Fatal("untrusted package was persisted as a candidate")
	}
}

func TestAdminApprovesVerifiedBuildPackageCandidate(t *testing.T) {
	roles := []string{"owner", "admin"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			manifestJSON, irJSON, wasmBytes := makeSignedBuildPackageParts(t)
			signature, err := pipeline.SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
			if err != nil {
				t.Fatal(err)
			}
			signatureJSON, err := pipeline.MarshalBuildPackageSignature(signature)
			if err != nil {
				t.Fatal(err)
			}
			archive := makeSignedBuildPackageZIP(t, map[string][]byte{
				"signature.json": signatureJSON,
				"manifest.json":  manifestJSON,
				"module.ir.json": irJSON,
				"module.wasm":    wasmBytes,
			})
			candidateRoot := filepath.Join(t.TempDir(), "candidates")
			candidates, err := packagecandidate.NewFileStore(candidateRoot)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := candidates.Put(context.Background(), seed.ShopID, archive)
			if err != nil {
				t.Fatal(err)
			}
			oldCandidates := moduleBuildPackageCandidates
			oldKeys := TrustedBuildPackageKeys()
			oldAuth := staffAuthenticator
			SetModuleBuildPackageCandidates(candidates)
			SetTrustedBuildPackageKeys(map[string]ed25519.PublicKey{signature.KeyID: publicKey})
			t.Cleanup(func() {
				SetModuleBuildPackageCandidates(oldCandidates)
				SetTrustedBuildPackageKeys(oldKeys)
				SetStaffAuthenticator(oldAuth)
			})
			mux := http.NewServeMux()
			Register(mux)
			auth := newCatalogTestAuthenticator(role, seed.ShopID)
			SetStaffAuthenticator(auth)
			path := "/v1/admin/modules/build-packages/candidates/" + receipt.SHA256

			pending := performAdminRequest(mux, http.MethodGet, path, auth.token, "")
			if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"status":"pending"`) {
				t.Fatalf("pending candidate response = %d/%s", pending.Code, pending.Body)
			}
			SetTrustedBuildPackageKeys(map[string]ed25519.PublicKey{})
			revoked := performAdminRequest(mux, http.MethodPost, path+"/approval", auth.token, `{"decision":"approve"}`)
			if revoked.Code != http.StatusConflict {
				t.Fatalf("approval after signer removal status = %d, want %d; body=%s", revoked.Code, http.StatusConflict, revoked.Body)
			}
			SetTrustedBuildPackageKeys(map[string]ed25519.PublicKey{signature.KeyID: publicKey})
			approved := performAdminRequest(mux, http.MethodPost, path+"/approval", auth.token, `{"decision":"approve"}`)
			if approved.Code != http.StatusOK || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
				t.Fatalf("approval response = %d/%s", approved.Code, approved.Body)
			}
			var response struct {
				Data struct {
					Approval packagecandidate.Approval `json:"approval"`
				} `json:"data"`
			}
			if err := json.Unmarshal(approved.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode approval response: %v", err)
			}
			if response.Data.Approval.ApprovedBy != "user-"+role || response.Data.Approval.ApproverRole != role {
				t.Fatalf("approval actor = %+v", response.Data.Approval)
			}

			repeated := performAdminRequest(mux, http.MethodPost, path+"/approval", auth.token, `{"decision":"approve"}`)
			if repeated.Code != http.StatusConflict {
				t.Fatalf("repeated approval status = %d, want %d; body=%s", repeated.Code, http.StatusConflict, repeated.Body)
			}
			otherShop := newCatalogTestAuthenticator(role, "e0e00000-0001-4000-8000-000000000099")
			SetStaffAuthenticator(otherShop)
			crossShop := performAdminRequest(mux, http.MethodGet, path, otherShop.token, "")
			if crossShop.Code != http.StatusNotFound {
				t.Fatalf("cross-shop candidate status = %d, want %d; body=%s", crossShop.Code, http.StatusNotFound, crossShop.Body)
			}
		})
	}
}

func TestBuildPackageCandidateApprovalRequiresAuthorizedAuthenticatedStaff(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, irJSON, wasmBytes := makeSignedBuildPackageParts(t)
	signature, err := pipeline.SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signatureJSON, err := pipeline.MarshalBuildPackageSignature(signature)
	if err != nil {
		t.Fatal(err)
	}
	archive := makeSignedBuildPackageZIP(t, map[string][]byte{
		"signature.json": signatureJSON,
		"manifest.json":  manifestJSON,
		"module.ir.json": irJSON,
		"module.wasm":    wasmBytes,
	})
	candidateRoot := filepath.Join(t.TempDir(), "candidates")
	candidates, err := packagecandidate.NewFileStore(candidateRoot)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := candidates.Put(context.Background(), seed.ShopID, archive)
	if err != nil {
		t.Fatal(err)
	}
	oldCandidates := moduleBuildPackageCandidates
	oldKeys := TrustedBuildPackageKeys()
	oldAuth := staffAuthenticator
	SetModuleBuildPackageCandidates(candidates)
	SetTrustedBuildPackageKeys(map[string]ed25519.PublicKey{signature.KeyID: publicKey})
	t.Cleanup(func() {
		SetModuleBuildPackageCandidates(oldCandidates)
		SetTrustedBuildPackageKeys(oldKeys)
		SetStaffAuthenticator(oldAuth)
	})
	mux := http.NewServeMux()
	Register(mux)
	path := "/v1/admin/modules/build-packages/candidates/" + receipt.SHA256 + "/approval"

	SetStaffAuthenticator(newCatalogTestAuthenticator("support", seed.ShopID))
	if response := performAdminRequest(mux, http.MethodPost, path, "catalog-test-support", `{"decision":"approve"}`); response.Code != http.StatusForbidden {
		t.Fatalf("support approval status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body)
	}
	SetStaffAuthenticator(nil)
	if response := performAdminRequest(mux, http.MethodPost, path, "", `{"decision":"approve"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous approval status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body)
	}
	SetStaffAuthenticator(newCatalogTestAuthenticator("admin", seed.ShopID))
	missing := performAdminRequest(mux, http.MethodGet, "/v1/admin/modules/build-packages/candidates/"+strings.Repeat("0", 64), "catalog-test-admin", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing candidate status = %d, want %d; body=%s", missing.Code, http.StatusNotFound, missing.Body)
	}
	invalidDigest := performAdminRequest(mux, http.MethodGet, "/v1/admin/modules/build-packages/candidates/not-a-digest", "catalog-test-admin", "")
	if invalidDigest.Code != http.StatusBadRequest {
		t.Fatalf("invalid digest status = %d, want %d; body=%s", invalidDigest.Code, http.StatusBadRequest, invalidDigest.Body)
	}
	shopHash := sha256.Sum256([]byte(seed.ShopID))
	candidatePath := filepath.Join(candidateRoot, hex.EncodeToString(shopHash[:]), receipt.SHA256)
	if err := os.WriteFile(candidatePath, []byte("tampered candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	tampered := performAdminRequest(mux, http.MethodPost, path, "catalog-test-admin", `{"decision":"approve"}`)
	if tampered.Code != http.StatusConflict {
		t.Fatalf("tampered candidate approval status = %d, want %d; body=%s", tampered.Code, http.StatusConflict, tampered.Body)
	}
	if approval, err := candidates.GetApproval(context.Background(), seed.ShopID, receipt.SHA256); !errors.Is(err, packagecandidate.ErrCandidateCorrupt) || approval != nil {
		t.Fatalf("tampered candidate approval = %+v, error %v", approval, err)
	}
}

func performSignedPackageImport(mux *http.ServeMux, token string, archive []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/modules/build-packages", bytes.NewReader(archive))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func makeSignedBuildPackageParts(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	source := []byte("test-source")
	sourceArchive := []byte("test-source-archive")
	provenance, err := pipeline.NewPackageProvenance(source, sourceArchive, pipeline.SourceProvenance{
		PlatformVersion: "unknown",
		ModuleVersion:   "1.0.0",
		Adapter:         "woocommerce",
		AdapterVersion:  "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	ir := pipeline.ModuleIR{
		Version:    pipeline.IRVersion,
		Platform:   "woocommerce",
		Module:     "demo.catalog-reader",
		Provenance: provenance,
		Triggers:   []pipeline.IRTrigger{{Kind: "hook", Name: "woocommerce_product_query"}},
		Operations: []pipeline.IROperation{{
			Call:  "catalog.product.get",
			Input: map[string]pipeline.IRValue{"product_id": {Literal: json.RawMessage(`"e0e00000-0001-4000-8000-000000000040"`)}},
		}},
	}
	wasmBytes, err := pipeline.CompileIRToWasm(ir)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := pipeline.NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	irJSON, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	return manifestJSON, irJSON, wasmBytes
}

func makeSignedBuildPackageZIP(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
