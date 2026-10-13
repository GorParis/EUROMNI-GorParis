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
}

func (s *Store) GetInstalledModule(ctx context.Context, shopID, moduleID string) (*InstalledModule, error) {
	if s == nil || s.SQL == nil {
		return nil, sql.ErrConnDone
	}
	var module InstalledModule
	var capabilitiesJSON string
	err := s.SQL.QueryRowContext(ctx, `
		SELECT m.id::text, m.name, m.abi_version, m.wasm_sha256,
		       array_to_json(sm.capabilities)::text
		FROM shop_modules sm
		JOIN modules m ON m.id = sm.module_id
		WHERE sm.shop_id::text = $1
		  AND sm.module_id::text = $2
		  AND sm.status = 'installed'
	`, shopID, moduleID).Scan(&module.ID, &module.Name, &module.ABIVersion, &module.WASMSHA256, &capabilitiesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &module.Capabilities); err != nil {
		return nil, err
	}
	return &module, nil
}
