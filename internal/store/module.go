package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"euromni/internal/abi"
)

type InstalledModule struct {
	ID           string
	Name         string
	ABIVersion   string
	WASMSHA256   []byte
	Capabilities []string
	Config       json.RawMessage
}

const NewProductsModuleName = "ps_newproducts"

var ErrModuleBuildUnsupported = errors.New("registered module build is not supported for installation")

type NewProductsSettings struct {
	Days  int `json:"days"`
	Limit int `json:"limit"`
}

type ModuleInventoryEntry struct {
	ID              string                      `json:"id"`
	Name            string                      `json:"name"`
	ABIVersion      string                      `json:"abi_version"`
	Platform        string                      `json:"platform,omitempty"`
	PlatformVersion string                      `json:"platform_version,omitempty"`
	ModuleVersion   string                      `json:"module_version,omitempty"`
	SourceReference string                      `json:"source_reference,omitempty"`
	Adapter         string                      `json:"adapter,omitempty"`
	AdapterVersion  string                      `json:"adapter_version,omitempty"`
	SourceSHA256    string                      `json:"source_sha256,omitempty"`
	ArchiveSHA256   string                      `json:"archive_sha256,omitempty"`
	WASMSHA256      string                      `json:"wasm_sha256,omitempty"`
	Capabilities    []string                    `json:"capabilities"`
	Status          string                      `json:"status"`
	CanInstall      bool                        `json:"can_install"`
	CanUninstall    bool                        `json:"can_uninstall"`
	Builds          []ModuleBuildInventoryEntry `json:"builds"`
}

type ModuleBuildInventoryEntry struct {
	WASMSHA256    string `json:"wasm_sha256"`
	ModuleVersion string `json:"module_version,omitempty"`
	Platform      string `json:"platform,omitempty"`
	Current       bool   `json:"current"`
	Installed     bool   `json:"installed"`
	CanPromote    bool   `json:"can_promote"`
}

type moduleBuildManifestMetadata struct {
	Version      string   `json:"version"`
	Module       string   `json:"module"`
	Platform     string   `json:"platform"`
	ABIVersion   string   `json:"abi_version"`
	Capabilities []string `json:"capabilities"`
	WASMSHA256   string   `json:"wasm_sha256"`
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

func (s *Store) ListModuleInventory(ctx context.Context, shopID string) ([]ModuleInventoryEntry, error) {
	if s == nil || s.SQL == nil {
		return nil, sql.ErrConnDone
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT m.id::text, m.name, m.abi_version,
		       COALESCE(b.wasm_sha256, m.wasm_sha256),
		       COALESCE(b.build_manifest::text, m.build_manifest::text), sm.status::text,
		       COALESCE(array_to_json(sm.capabilities)::text, '[]')
		FROM modules m
		LEFT JOIN shop_modules sm ON sm.module_id = m.id AND sm.shop_id::text = $1
		LEFT JOIN module_builds b
		  ON b.module_id = m.id
		 AND b.id = CASE WHEN sm.status = 'installed'
		                 THEN sm.build_id
		                 ELSE m.current_build_id END
		ORDER BY m.name
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]ModuleInventoryEntry, 0)
	for rows.Next() {
		var entry ModuleInventoryEntry
		var wasmHash []byte
		var manifestJSON sql.NullString
		var status sql.NullString
		var capabilitiesJSON string
		if err := rows.Scan(&entry.ID, &entry.Name, &entry.ABIVersion, &wasmHash,
			&manifestJSON, &status, &capabilitiesJSON); err != nil {
			return nil, err
		}
		entry.Status = "registered"
		if status.Valid {
			entry.Status = status.String
		}
		if err := json.Unmarshal([]byte(capabilitiesJSON), &entry.Capabilities); err != nil {
			return nil, fmt.Errorf("decode capabilities for module %q: %w", entry.Name, err)
		}
		if len(wasmHash) == 32 {
			entry.WASMSHA256 = hex.EncodeToString(wasmHash)
		}
		if manifestJSON.Valid {
			var manifest moduleBuildManifestMetadata
			if err := json.Unmarshal([]byte(manifestJSON.String), &manifest); err != nil {
				return nil, fmt.Errorf("decode build manifest for module %q: %w", entry.Name, err)
			}
			entry.Platform = manifest.Platform
			entry.PlatformVersion = manifest.Source.PlatformVersion
			entry.ModuleVersion = manifest.Source.ModuleVersion
			entry.SourceReference = manifest.Source.SourceReference
			entry.Adapter = manifest.Source.Adapter
			entry.AdapterVersion = manifest.Source.AdapterVersion
			entry.SourceSHA256 = manifest.Source.SourceSHA256
			entry.ArchiveSHA256 = manifest.Source.ArchiveSHA256
			if entry.Status == "registered" {
				entry.Capabilities = append([]string(nil), manifest.Capabilities...)
			}
			entry.CanInstall = moduleBuildCanInstall(entry.Name, entry.ABIVersion, wasmHash, manifest)
		}
		entry.CanUninstall = entry.Name == NewProductsModuleName && entry.Status == "installed"
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.populateModuleBuildInventory(ctx, shopID, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) populateModuleBuildInventory(ctx context.Context, shopID string, entries []ModuleInventoryEntry) error {
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT b.module_id::text, b.wasm_sha256, b.build_manifest::text,
		       b.id = m.current_build_id,
		       b.id = sm.build_id AND sm.status = 'installed',
		       b.id = m.current_build_id OR EXISTS (
		           SELECT 1 FROM shop_modules pinned WHERE pinned.build_id = b.id
		       )
		FROM module_builds b
		JOIN modules m ON m.id = b.module_id
		LEFT JOIN shop_modules sm ON sm.module_id = m.id AND sm.shop_id::text = $1
		WHERE b.id = m.current_build_id OR EXISTS (
		    SELECT 1 FROM shop_modules pinned WHERE pinned.build_id = b.id
		)
		ORDER BY b.module_id, b.created_at DESC, b.id
	`, shopID)
	if err != nil {
		return err
	}
	defer rows.Close()

	entryByID := make(map[string]int, len(entries))
	for i := range entries {
		entryByID[entries[i].ID] = i
	}
	for rows.Next() {
		var moduleID string
		var wasmHash []byte
		var manifestRaw sql.NullString
		var current, installed, retained bool
		if err := rows.Scan(&moduleID, &wasmHash, &manifestRaw, &current, &installed, &retained); err != nil {
			return err
		}
		index, exists := entryByID[moduleID]
		if !exists {
			continue
		}
		if len(wasmHash) != 32 {
			return ErrInvalidModuleWASMHash
		}
		build := ModuleBuildInventoryEntry{
			WASMSHA256: hex.EncodeToString(wasmHash),
			Current:    current,
			Installed:  installed,
		}
		if manifestRaw.Valid {
			var manifest moduleBuildManifestMetadata
			if err := json.Unmarshal([]byte(manifestRaw.String), &manifest); err != nil {
				return fmt.Errorf("decode build manifest for module %q: %w", entries[index].Name, err)
			}
			build.ModuleVersion = manifest.Source.ModuleVersion
			build.Platform = manifest.Platform
			build.CanPromote = retained &&
				moduleBuildCanInstall(entries[index].Name, entries[index].ABIVersion, wasmHash, manifest)
		}
		entries[index].Builds = append(entries[index].Builds, build)
	}
	return rows.Err()
}

func (s *Store) InstallRegisteredNewProductsModule(ctx context.Context, shopID, expectedWASMHash string) error {
	if s == nil || s.SQL == nil {
		return sql.ErrConnDone
	}
	shopID, err := abi.RequireUUID("shop_id", shopID)
	if err != nil {
		return err
	}
	if !isSHA256Hex(expectedWASMHash) {
		return ErrModuleBuildUnsupported
	}
	expectedWASMHashBytes, _ := hex.DecodeString(expectedWASMHash)
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var moduleID, buildID, abiVersion string
	var wasmHash []byte
	var manifestRaw sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT m.id::text, b.id::text, m.abi_version, b.wasm_sha256, b.build_manifest::text
		FROM modules m
		JOIN module_builds b ON b.module_id = m.id AND b.id = m.current_build_id
		WHERE m.name = $1 AND b.wasm_sha256 = $2
		FOR UPDATE OF m
	`, NewProductsModuleName, expectedWASMHashBytes).Scan(&moduleID, &buildID, &abiVersion, &wasmHash, &manifestRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !manifestRaw.Valid || hex.EncodeToString(wasmHash) != expectedWASMHash {
		return ErrModuleBuildUnsupported
	}
	var manifest moduleBuildManifestMetadata
	if err := json.Unmarshal([]byte(manifestRaw.String), &manifest); err != nil {
		return fmt.Errorf("decode registered module build manifest: %w", err)
	}
	if !moduleBuildCanInstall(NewProductsModuleName, abiVersion, wasmHash, manifest) ||
		manifest.WASMSHA256 != expectedWASMHash {
		return ErrModuleBuildUnsupported
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO shop_modules (shop_id, module_id, build_id, status, capabilities, config)
		VALUES ($1, $2, $3, 'installed', ARRAY['catalog']::text[], '{"days":30,"limit":8}'::jsonb)
		ON CONFLICT (shop_id, module_id) DO UPDATE
		SET status = 'installed',
		    build_id = EXCLUDED.build_id,
		    capabilities = EXCLUDED.capabilities,
		    config = CASE
		        WHEN shop_modules.config ? 'days' AND shop_modules.config ? 'limit'
		        THEN shop_modules.config
		        ELSE shop_modules.config || EXCLUDED.config
		    END,
		    updated_at = now()
	`, shopID, moduleID, buildID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UninstallNewProductsModule(ctx context.Context, shopID string) error {
	if s == nil || s.SQL == nil {
		return sql.ErrConnDone
	}
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE shop_modules sm
		SET status = 'disabled', updated_at = now()
		FROM modules m
		WHERE sm.module_id = m.id
		  AND sm.shop_id::text = $1
		  AND m.name = $2
		  AND sm.status = 'installed'
	`, shopID, NewProductsModuleName)
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

func moduleBuildCanInstall(name, abiVersion string, wasmHash []byte, manifest moduleBuildManifestMetadata) bool {
	return name == NewProductsModuleName &&
		abiVersion == abi.Version &&
		manifest.Version == "euromni.build.v1" &&
		manifest.Module == name &&
		manifest.Platform == "prestashop" &&
		manifest.ABIVersion == abi.Version &&
		len(wasmHash) == 32 &&
		manifest.WASMSHA256 == hex.EncodeToString(wasmHash) &&
		len(manifest.Capabilities) == 1 &&
		manifest.Capabilities[0] == "catalog" &&
		isSHA256Hex(manifest.Source.SourceSHA256) &&
		isSHA256Hex(manifest.Source.ArchiveSHA256) &&
		manifest.Source.ModuleVersion != "" &&
		manifest.Source.PlatformVersion != "" &&
		manifest.Source.Adapter != "" &&
		manifest.Source.AdapterVersion != ""
}

func isSHA256Hex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (s *Store) GetNewProductsSettings(ctx context.Context, shopID string) (NewProductsSettings, error) {
	if s == nil || s.SQL == nil {
		return NewProductsSettings{}, sql.ErrConnDone
	}
	var configJSON string
	err := s.SQL.QueryRowContext(ctx, `
		SELECT sm.config::text
		FROM shop_modules sm
		JOIN modules m ON m.id = sm.module_id
		WHERE sm.shop_id::text = $1
		  AND m.name = $2
		  AND sm.status = 'installed'
	`, shopID, NewProductsModuleName).Scan(&configJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return NewProductsSettings{}, ErrNotFound
	}
	if err != nil {
		return NewProductsSettings{}, err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil || config == nil {
		return NewProductsSettings{}, errors.New("installed module configuration is invalid")
	}
	settings, err := decodeNewProductsSettings(config)
	if err != nil {
		return NewProductsSettings{}, err
	}
	if err := ValidateNewProductsQuery(settings.Days, settings.Limit); err != nil {
		return NewProductsSettings{}, err
	}
	return settings, nil
}

func (s *Store) UpdateNewProductsSettings(ctx context.Context, shopID string, settings NewProductsSettings) error {
	if s == nil || s.SQL == nil {
		return sql.ErrConnDone
	}
	if err := ValidateNewProductsQuery(settings.Days, settings.Limit); err != nil {
		return err
	}
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE shop_modules sm
		SET config = sm.config || jsonb_build_object('days', $3::int, 'limit', $4::int),
		    updated_at = now()
		FROM modules m
		WHERE sm.module_id = m.id
		  AND sm.shop_id::text = $1
		  AND m.name = $2
		  AND sm.status = 'installed'
	`, shopID, NewProductsModuleName, settings.Days, settings.Limit)
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
	return nil
}

func decodeNewProductsSettings(config map[string]json.RawMessage) (NewProductsSettings, error) {
	var settings NewProductsSettings
	var days *int
	var limit *int
	if err := json.Unmarshal(config["days"], &days); err != nil || days == nil {
		return settings, errors.New("installed module configuration is missing days")
	}
	if err := json.Unmarshal(config["limit"], &limit); err != nil || limit == nil {
		return settings, errors.New("installed module configuration is missing limit")
	}
	settings.Days = *days
	settings.Limit = *limit
	return settings, nil
}

func (s *Store) GetInstalledModule(ctx context.Context, shopID, moduleID string) (*InstalledModule, error) {
	if s == nil || s.SQL == nil {
		return nil, sql.ErrConnDone
	}
	var module InstalledModule
	var capabilitiesJSON string
	var configJSON string
	err := s.SQL.QueryRowContext(ctx, `
		SELECT m.id::text, m.name, m.abi_version, b.wasm_sha256,
		       array_to_json(sm.capabilities)::text, sm.config::text
		FROM shop_modules sm
		JOIN modules m ON m.id = sm.module_id
		LEFT JOIN module_builds b
		  ON b.module_id = m.id AND b.id = sm.build_id
		WHERE sm.shop_id::text = $1
		  AND sm.module_id::text = $2
		  AND sm.status = 'installed'
	`, shopID, moduleID).Scan(&module.ID, &module.Name, &module.ABIVersion, &module.WASMSHA256, &capabilitiesJSON, &configJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &module.Capabilities); err != nil {
		return nil, err
	}
	module.Config = json.RawMessage(configJSON)
	return &module, nil
}

func (s *Store) GetInstalledModuleByName(ctx context.Context, shopID, name string) (*InstalledModule, error) {
	if s == nil || s.SQL == nil {
		return nil, sql.ErrConnDone
	}
	var module InstalledModule
	var capabilitiesJSON string
	var configJSON string
	err := s.SQL.QueryRowContext(ctx, `
		SELECT m.id::text, m.name, m.abi_version, b.wasm_sha256,
		       array_to_json(sm.capabilities)::text, sm.config::text
		FROM shop_modules sm
		JOIN modules m ON m.id = sm.module_id
		LEFT JOIN module_builds b
		  ON b.module_id = m.id AND b.id = sm.build_id
		WHERE sm.shop_id::text = $1
		  AND m.name = $2
		  AND sm.status = 'installed'
	`, shopID, name).Scan(&module.ID, &module.Name, &module.ABIVersion, &module.WASMSHA256, &capabilitiesJSON, &configJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &module.Capabilities); err != nil {
		return nil, err
	}
	module.Config = json.RawMessage(configJSON)
	return &module, nil
}
