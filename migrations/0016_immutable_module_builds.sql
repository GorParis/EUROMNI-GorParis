CREATE TABLE module_builds (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  module_id    uuid NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
  wasm_sha256  bytea NOT NULL CHECK (octet_length(wasm_sha256) = 32),
  build_manifest jsonb,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (module_id, wasm_sha256),
  UNIQUE (module_id, id),
  CHECK (build_manifest IS NULL OR jsonb_typeof(build_manifest) = 'object')
);

INSERT INTO module_builds (module_id, wasm_sha256, build_manifest)
SELECT id, wasm_sha256, build_manifest
FROM modules
WHERE wasm_sha256 IS NOT NULL;

ALTER TABLE modules
  ADD COLUMN current_build_id uuid,
  ADD CONSTRAINT modules_current_build_fk
    FOREIGN KEY (current_build_id) REFERENCES module_builds (id) ON DELETE SET NULL;

UPDATE modules m
SET current_build_id = b.id
FROM module_builds b
WHERE b.module_id = m.id AND b.wasm_sha256 = m.wasm_sha256;

ALTER TABLE shop_modules
  ADD COLUMN build_id uuid,
  ADD CONSTRAINT shop_modules_build_fk
    FOREIGN KEY (module_id, build_id) REFERENCES module_builds (module_id, id);

UPDATE shop_modules sm
SET build_id = m.current_build_id
FROM modules m
WHERE sm.module_id = m.id AND m.current_build_id IS NOT NULL;
