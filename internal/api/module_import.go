package api

import (
	"errors"
	"io"
	"log"
	"mime"
	"net/http"

	"euromni/internal/packagecandidate"
	"euromni/internal/pipeline"
	"euromni/internal/quarantine"
)

var moduleArchiveQuarantine quarantine.Store
var moduleBuildPackageCandidates packagecandidate.Store

func SetModuleArchiveQuarantine(store quarantine.Store) {
	moduleArchiveQuarantine = store
}

func SetModuleBuildPackageCandidates(store packagecandidate.Store) {
	moduleBuildPackageCandidates = store
}

func adminQuarantineModuleArchive(w http.ResponseWriter, r *http.Request) {
	if moduleArchiveQuarantine == nil {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "module quarantine unavailable"})
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/zip" {
		writePrivateJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "content type must be application/zip"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, pipeline.MaxPHPArchiveBytes)
	defer r.Body.Close()
	archive, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "module archive exceeds size limit"})
			return
		}
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "module archive could not be read"})
		return
	}
	if _, err := pipeline.ParsePHPManifest(archive); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "module archive rejected"})
		return
	}
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	quarantined, err := moduleArchiveQuarantine.Put(r.Context(), session.ShopID, archive)
	if err != nil {
		switch {
		case errors.Is(err, quarantine.ErrArchiveEmpty):
			writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "module archive is empty"})
		case errors.Is(err, quarantine.ErrArchiveLarge):
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "module archive exceeds size limit"})
		case errors.Is(err, quarantine.ErrQuarantineFull):
			writePrivateJSON(w, http.StatusInsufficientStorage, map[string]any{"error": "module quarantine is full"})
		default:
			log.Printf("admin module quarantine: %v", err)
			writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "module archive could not be quarantined"})
		}
		return
	}
	writePrivateJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"sha256": quarantined.SHA256,
			"size":   quarantined.Size,
			"status": "quarantined",
		},
	})
}
