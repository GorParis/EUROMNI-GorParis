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
	"sort"
	"strings"

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
		SELECT DISTINCT wasm_sha256
		FROM (
			SELECT b.wasm_sha256
			FROM module_builds b
			JOIN modules m ON m.current_build_id = b.id
			UNION ALL
			SELECT b.wasm_sha256
			FROM module_builds b
			JOIN shop_modules sm ON sm.build_id = b.id
			UNION ALL
			SELECT wasm_sha256 FROM modules
			WHERE wasm_sha256 IS NOT NULL AND current_build_id IS NULL
		) referenced_builds
		ORDER BY wasm_sha256
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
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var previousBuildID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT current_build_id::text
		FROM modules
		WHERE id = $1
		FOR UPDATE
	`, moduleID).Scan(&previousBuildID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	buildID, err := registerImmutableBuild(ctx, tx, moduleID, wasmSHA256, manifestJSON)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE modules
		SET current_build_id = $2, wasm_sha256 = $3, build_manifest = $4::jsonb
		WHERE id = $1
	`, moduleID, buildID, wasmSHA256, string(manifestJSON)); err != nil {
		return err
	}
	if !previousBuildID.Valid {
		if _, err := tx.ExecContext(ctx, `
			UPDATE shop_modules SET build_id = $2
			WHERE module_id = $1 AND build_id IS NULL
		`, moduleID, buildID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PromoteInstalledModuleBuild(ctx context.Context, shopID, moduleID, expectedWASMHash string) error {
	if s == nil || s.SQL == nil {
		return sql.ErrConnDone
	}
	shopID, err := abi.RequireUUID("shop_id", shopID)
	if err != nil {
		return err
	}
	moduleID, err = abi.RequireUUID("module_id", moduleID)
	if err != nil {
		return err
	}
	if len(expectedWASMHash) != sha256.Size*2 {
		return ErrInvalidModuleWASMHash
	}
	wasmHash, err := hex.DecodeString(expectedWASMHash)
	if err != nil {
		return ErrInvalidModuleWASMHash
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var buildID string
	err = tx.QueryRowContext(ctx, `
		SELECT b.id::text
		FROM module_builds b
		JOIN modules m ON m.id = b.module_id
		WHERE b.module_id = $1 AND b.wasm_sha256 = $2
		  AND (m.current_build_id = b.id OR EXISTS (
		      SELECT 1 FROM shop_modules sm WHERE sm.build_id = b.id
		  ))
	`, moduleID, wasmHash).Scan(&buildID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE shop_modules
		SET build_id = $3, updated_at = now()
		WHERE shop_id = $1 AND module_id = $2 AND status = 'installed'
	`, shopID, moduleID, buildID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) InstallNewProductsModuleBuild(ctx context.Context, shopID string, wasmSHA256, manifestJSON []byte, settings NewProductsSettings) (string, error) {
	if s == nil || s.SQL == nil {
		return "", sql.ErrConnDone
	}
	shopID, err := abi.RequireUUID("shop_id", shopID)
	if err != nil {
		return "", err
	}
	if err := ValidateNewProductsQuery(settings.Days, settings.Limit); err != nil {
		return "", err
	}
	if err := validateBuildManifestJSON(wasmSHA256, manifestJSON); err != nil {
		return "", err
	}
	var manifest struct {
		Module     string `json:"module"`
		Platform   string `json:"platform"`
		ABIVersion string `json:"abi_version"`
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil ||
		manifest.Module != NewProductsModuleName || manifest.Platform != "prestashop" || manifest.ABIVersion != abi.Version {
		return "", ErrInvalidBuildManifest
	}
	configJSON, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}

	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var moduleID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO modules (name, abi_version, wasm_sha256, build_manifest)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (name) DO UPDATE
		SET abi_version = EXCLUDED.abi_version
		RETURNING id::text
	`, NewProductsModuleName, abi.Version, nil, nil).Scan(&moduleID)
	if err != nil {
		return "", err
	}
	buildID, err := registerImmutableBuild(ctx, tx, moduleID, wasmSHA256, manifestJSON)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE modules
		SET current_build_id = $2, wasm_sha256 = $3, build_manifest = $4::jsonb
		WHERE id = $1
	`, moduleID, buildID, wasmSHA256, string(manifestJSON)); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO shop_modules (shop_id, module_id, build_id, status, capabilities, config)
		VALUES ($1, $2, $3, 'installed', ARRAY['catalog']::text[], $4::jsonb)
		ON CONFLICT (shop_id, module_id) DO UPDATE
		SET status = 'installed',
		    build_id = EXCLUDED.build_id,
		    capabilities = EXCLUDED.capabilities,
		    config = shop_modules.config || jsonb_build_object(
		        'days', COALESCE(shop_modules.config->'days', EXCLUDED.config->'days'),
		        'limit', COALESCE(shop_modules.config->'limit', EXCLUDED.config->'limit')
		    ),
		    updated_at = now()
	`, shopID, moduleID, buildID, string(configJSON))
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return moduleID, nil
}

func registerImmutableBuild(ctx context.Context, tx *sql.Tx, moduleID string, wasmSHA256, manifestJSON []byte) (string, error) {
	buildID, err := NewCatalogID()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO module_builds (id, module_id, wasm_sha256, build_manifest)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (module_id, wasm_sha256) DO NOTHING
	`, buildID, moduleID, wasmSHA256, string(manifestJSON)); err != nil {
		return "", err
	}
	var storedID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM module_builds
		WHERE module_id = $1 AND wasm_sha256 = $2
		  AND build_manifest = $3::jsonb
	`, moduleID, wasmSHA256, string(manifestJSON)).Scan(&storedID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: an existing Wasm digest has different provenance", ErrInvalidBuildManifest)
	}
	if err != nil {
		return "", err
	}
	return storedID, nil
}

func validateBuildManifestJSON(wasmSHA256, manifestJSON []byte) error {
	if len(wasmSHA256) != sha256.Size || len(manifestJSON) == 0 {
		return ErrInvalidBuildManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestJSON))
	decoder.DisallowUnknownFields()
	var manifest struct {
		Version      string   `json:"version"`
		Module       string   `json:"module"`
		Platform     string   `json:"platform"`
		IRVersion    string   `json:"ir_version"`
		IRSHA256     string   `json:"ir_sha256"`
		ABIVersion   string   `json:"abi_version"`
		Compiler     string   `json:"compiler"`
		Capabilities []string `json:"capabilities"`
		WASMSHA256   string   `json:"wasm_sha256"`
		WASMSize     int64    `json:"wasm_size"`
		Source       struct {
			SourceSHA256    string `json:"source_sha256"`
			ArchiveSHA256   string `json:"archive_sha256"`
			SourceReference string `json:"source_reference"`
			ModuleVersion   string `json:"module_version"`
			PlatformVersion string `json:"platform_version"`
			Adapter         string `json:"adapter"`
			AdapterVersion  string `json:"adapter_version"`
		} `json:"source"`
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
	if manifest.Version != "euromni.build.v1" ||
		!nonEmptyManifestValue(manifest.Module) ||
		!nonEmptyManifestValue(manifest.Platform) ||
		!nonEmptyManifestValue(manifest.IRVersion) ||
		!isManifestSHA256(manifest.IRSHA256) ||
		manifest.ABIVersion != abi.Version ||
		!nonEmptyManifestValue(manifest.Compiler) ||
		manifest.WASMSize <= 0 ||
		!isManifestSHA256(manifest.Source.SourceSHA256) ||
		!isManifestSHA256(manifest.Source.ArchiveSHA256) ||
		!nonEmptyManifestValue(manifest.Source.PlatformVersion) ||
		!nonEmptyManifestValue(manifest.Source.Adapter) ||
		!nonEmptyManifestValue(manifest.Source.AdapterVersion) ||
		len(manifest.Capabilities) == 0 ||
		!sort.StringsAreSorted(manifest.Capabilities) {
		return fmt.Errorf("%w: required provenance or toolchain fields are missing or invalid", ErrInvalidBuildManifest)
	}
	for index, capability := range manifest.Capabilities {
		if !nonEmptyManifestValue(capability) ||
			(index > 0 && manifest.Capabilities[index-1] == capability) {
			return fmt.Errorf("%w: capabilities must be nonempty, unique, and sorted", ErrInvalidBuildManifest)
		}
	}
	return nil
}

func nonEmptyManifestValue(value string) bool {
	return strings.TrimSpace(value) == value && value != ""
}

func isManifestSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}
