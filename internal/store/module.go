package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

type NewProductsSettings struct {
	Days  int `json:"days"`
	Limit int `json:"limit"`
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
		SELECT m.id::text, m.name, m.abi_version, m.wasm_sha256,
		       array_to_json(sm.capabilities)::text, sm.config::text
		FROM shop_modules sm
		JOIN modules m ON m.id = sm.module_id
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
