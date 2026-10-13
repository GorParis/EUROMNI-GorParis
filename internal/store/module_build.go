package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"euromni/internal/abi"
)

var ErrInvalidBuildManifest = errors.New("invalid module build manifest")
var ErrInvalidModuleWASMHash = errors.New("module has an invalid Wasm hash")

func (s *Store) ListModuleWASMDigests(ctx context.Context) ([]string, error) {
	if s == nil || s.SQL == nil {
		return nil, sql.ErrConnDone
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT wasm_sha256
		FROM modules
		WHERE wasm_sha256 IS NOT NULL
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	digests := make([]string, 0)
	for rows.Next() {
		var digest []byte
		if err := rows.Scan(&digest); err != nil {
			return nil, err
		}
		if len(digest) != sha256.Size {
			return nil, ErrInvalidModuleWASMHash
		}
		digests = append(digests, hex.EncodeToString(digest))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return digests, nil
}

func (s *Store) RegisterModuleBuild(ctx context.Context, moduleID string, wasmSHA256, manifestJSON []byte) error {
	if s == nil || s.SQL == nil {
		return sql.ErrConnDone
	}
	moduleID, err := abi.RequireUUID("module_id", moduleID)
	if err != nil {
		return err
	}
	if err := validateBuildManifestJSON(wasmSHA256, manifestJSON); err != nil {
		return err
	}
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE modules
		SET wasm_sha256 = $2, build_manifest = $3::jsonb
		WHERE id = $1
	`, moduleID, wasmSHA256, string(manifestJSON))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrNotFound
	}
	return nil
}

func validateBuildManifestJSON(wasmSHA256, manifestJSON []byte) error {
	if len(wasmSHA256) != sha256.Size || len(manifestJSON) == 0 {
		return ErrInvalidBuildManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestJSON))
	var manifest struct {
		WASMSHA256 string `json:"wasm_sha256"`
	}
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("%w: malformed JSON", ErrInvalidBuildManifest)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON data", ErrInvalidBuildManifest)
	}
	if manifest.WASMSHA256 != hex.EncodeToString(wasmSHA256) {
		return fmt.Errorf("%w: Wasm hash mismatch", ErrInvalidBuildManifest)
	}
	return nil
}
