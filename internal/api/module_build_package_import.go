package api

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"

	"euromni/internal/packagecandidate"
	"euromni/internal/pipeline"
)

var signedBuildPackageImportSlot = make(chan struct{}, 1)
var (
	errBuildPackageCandidateStorageUnavailable = errors.New("build package candidate storage unavailable")
	errBuildPackageCandidateTrustUnavailable   = errors.New("trusted build package keys unavailable")
	errBuildPackageCandidateDigestInvalid      = errors.New("build package candidate digest invalid")
	errBuildPackageCandidateVerification       = errors.New("stored build package candidate failed validation")
)

func adminImportSignedBuildPackage(w http.ResponseWriter, r *http.Request) {
	if moduleBuildPackageCandidates == nil {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "build package candidate storage unavailable"})
		return
	}
	select {
	case signedBuildPackageImportSlot <- struct{}{}:
		defer func() { <-signedBuildPackageImportSlot }()
	default:
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "another build package import is in progress"})
		return
	}
	trustedKeys := TrustedBuildPackageKeys()
	if trustedKeys == nil {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "trusted build package keys are not configured"})
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/zip" {
		writePrivateJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "content type must be application/zip"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, pipeline.MaxBuildPackageArchiveBytes)
	defer r.Body.Close()
	archiveBytes, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "build package exceeds transfer size limit"})
			return
		}
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "build package could not be read"})
		return
	}
	packageArchive, err := pipeline.ReadSignedBuildPackageArchive(archiveBytes)
	if err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "build package archive rejected"})
		return
	}
	manifest, err := pipeline.ValidateSignedBuildPackage(
		packageArchive.SignatureJSON, packageArchive.ManifestJSON,
		packageArchive.IRJSON, packageArchive.WASMBytes, trustedKeys,
	)
	if err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "build package signature or contents rejected"})
		return
	}
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	candidate, err := moduleBuildPackageCandidates.Put(r.Context(), session.ShopID, archiveBytes)
	if err != nil {
		switch {
		case errors.Is(err, packagecandidate.ErrCandidateEmpty):
			writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "build package is empty"})
		case errors.Is(err, packagecandidate.ErrCandidateLarge):
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "build package exceeds transfer size limit"})
		case errors.Is(err, packagecandidate.ErrCandidateStoreFull):
			writePrivateJSON(w, http.StatusInsufficientStorage, map[string]any{"error": "shop build package candidate storage is full"})
		default:
			log.Printf("admin signed build package candidate: %v", err)
			writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "build package candidate could not be stored"})
		}
		return
	}
	writePrivateJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"package_sha256": candidate.SHA256,
			"size":           candidate.Size,
			"status":         "candidate",
			"module":         manifest.Module,
			"platform":       manifest.Platform,
			"wasm_sha256":    manifest.WASMSHA256,
			"signer_key_id":  packageArchive.Signature.KeyID,
		},
	})
}

func adminBuildPackageCandidate(w http.ResponseWriter, r *http.Request) {
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	digest := r.PathValue("packageSHA256")
	manifest, signature, err := loadVerifiedBuildPackageCandidate(r, session.ShopID, digest)
	if err != nil {
		writeBuildPackageCandidateError(w, err)
		return
	}
	review, err := moduleBuildPackageCandidates.GetReview(r.Context(), session.ShopID, digest)
	if err != nil {
		writeBuildPackageCandidateError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"package_sha256":   digest,
			"status":           review.Status,
			"module":           manifest.Module,
			"platform":         manifest.Platform,
			"wasm_sha256":      manifest.WASMSHA256,
			"signer_key_id":    signature.KeyID,
			"approval":         review.Approval,
			"rejection":        review.Rejection,
			"decision_history": review.History,
		},
	})
}

func adminBuildPackageCandidates(w http.ResponseWriter, r *http.Request) {
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	if moduleBuildPackageCandidates == nil {
		writeBuildPackageCandidateError(w, errBuildPackageCandidateStorageUnavailable)
		return
	}
	limit, offset, err := parseBuildPackageCandidatePagination(r)
	if err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	items, total, err := moduleBuildPackageCandidates.List(r.Context(), session.ShopID, limit, offset)
	if err != nil {
		writeBuildPackageCandidateError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":  items,
			"limit":  limit,
			"offset": offset,
			"total":  total,
		},
	})
}

func adminApproveBuildPackageCandidate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Decision string `json:"decision"`
	}
	if err := decodeAdminJSON(w, r, &request); err != nil || request.Decision != "approve" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": `body must contain {"decision":"approve"}`})
		return
	}
	adminDecideBuildPackageCandidate(w, r, packagecandidate.DecisionApprove, "")
}

func adminRejectBuildPackageCandidate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := decodeAdminJSON(w, r, &request); err != nil || request.Decision != packagecandidate.DecisionReject {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": `body must contain {"decision":"reject","reason":"…"}`})
		return
	}
	adminDecideBuildPackageCandidate(w, r, packagecandidate.DecisionReject, request.Reason)
}

func adminReopenBuildPackageCandidate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Decision string `json:"decision"`
	}
	if err := decodeAdminJSON(w, r, &request); err != nil || request.Decision != packagecandidate.DecisionReopen {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": `body must contain {"decision":"reopen"}`})
		return
	}
	adminDecideBuildPackageCandidate(w, r, packagecandidate.DecisionReopen, "")
}

func adminDecideBuildPackageCandidate(w http.ResponseWriter, r *http.Request, decision, reason string) {
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	digest := r.PathValue("packageSHA256")
	if _, _, err := loadVerifiedBuildPackageCandidate(r, session.ShopID, digest); err != nil {
		writeBuildPackageCandidateError(w, err)
		return
	}
	review, err := moduleBuildPackageCandidates.Decide(
		r.Context(), session.ShopID, digest, session.UserID, session.Role, decision, reason,
	)
	if err != nil {
		writeBuildPackageDecisionError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": review})
}

func writeBuildPackageDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, packagecandidate.ErrCandidateDecisionConflict),
		errors.Is(err, packagecandidate.ErrCandidateApproved),
		errors.Is(err, packagecandidate.ErrCandidateDecisionHistoryFull):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "build package candidate decision conflicts with its current state"})
	case errors.Is(err, packagecandidate.ErrCandidateReasonRequired):
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "a rejection reason is required"})
	case errors.Is(err, packagecandidate.ErrCandidateReasonInvalid):
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "rejection reason exceeds the allowed size"})
	case errors.Is(err, packagecandidate.ErrCandidateNotFound),
		errors.Is(err, packagecandidate.ErrCandidateCorrupt):
		writeBuildPackageCandidateError(w, err)
	default:
		log.Printf("admin build package candidate decision: %v", err)
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "build package candidate decision failed"})
	}
}

func parseBuildPackageCandidatePagination(r *http.Request) (int, int, error) {
	limit := 50
	offset := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, errors.New("limit must be between 1 and 100")
		}
		limit = value
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100000 {
			return 0, 0, errors.New("offset must be between 0 and 100000")
		}
		offset = value
	}
	return limit, offset, nil
}

func loadVerifiedBuildPackageCandidate(r *http.Request, shopID, digest string) (pipeline.BuildManifest, pipeline.BuildPackageSignature, error) {
	if moduleBuildPackageCandidates == nil {
		return pipeline.BuildManifest{}, pipeline.BuildPackageSignature{}, errBuildPackageCandidateStorageUnavailable
	}
	if !isSHA256Hex(digest) {
		return pipeline.BuildManifest{}, pipeline.BuildPackageSignature{}, errBuildPackageCandidateDigestInvalid
	}
	trustedKeys := TrustedBuildPackageKeys()
	if trustedKeys == nil {
		return pipeline.BuildManifest{}, pipeline.BuildPackageSignature{}, errBuildPackageCandidateTrustUnavailable
	}
	archiveBytes, err := moduleBuildPackageCandidates.Get(r.Context(), shopID, digest)
	if err != nil {
		return pipeline.BuildManifest{}, pipeline.BuildPackageSignature{}, err
	}
	packageArchive, err := pipeline.ReadSignedBuildPackageArchive(archiveBytes)
	if err != nil {
		return pipeline.BuildManifest{}, pipeline.BuildPackageSignature{}, fmt.Errorf("%w: archive: %v", errBuildPackageCandidateVerification, err)
	}
	manifest, err := pipeline.ValidateSignedBuildPackage(
		packageArchive.SignatureJSON, packageArchive.ManifestJSON,
		packageArchive.IRJSON, packageArchive.WASMBytes, trustedKeys,
	)
	if err != nil {
		return pipeline.BuildManifest{}, pipeline.BuildPackageSignature{}, fmt.Errorf("%w: signature or contents: %v", errBuildPackageCandidateVerification, err)
	}
	return manifest, packageArchive.Signature, nil
}

func writeBuildPackageCandidateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, packagecandidate.ErrCandidateNotFound):
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "build package candidate not found"})
	case errors.Is(err, packagecandidate.ErrCandidateCorrupt):
		log.Printf("admin build package candidate integrity check: %v", err)
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "build package candidate failed integrity verification"})
	case errors.Is(err, packagecandidate.ErrCandidateApproved):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "build package candidate is already approved"})
	case errors.Is(err, errBuildPackageCandidateDigestInvalid):
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "package digest must be a lowercase SHA-256"})
	case errors.Is(err, errBuildPackageCandidateStorageUnavailable), errors.Is(err, errBuildPackageCandidateTrustUnavailable):
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "build package candidate review is unavailable"})
	case errors.Is(err, errBuildPackageCandidateVerification):
		log.Printf("admin build package candidate verification: %v", err)
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "build package candidate no longer passes verification"})
	default:
		log.Printf("admin build package candidate review: %v", err)
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "build package candidate review failed"})
	}
}
