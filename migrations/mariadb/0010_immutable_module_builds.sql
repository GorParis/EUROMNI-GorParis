CREATE TABLE module_builds (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  module_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  wasm_sha256 BINARY(32) NOT NULL,
  build_manifest JSON,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY module_builds_module_hash_unique (module_id, wasm_sha256),
  UNIQUE KEY module_builds_module_id_unique (module_id, id),
  CONSTRAINT fk_module_builds_module FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT INTO module_builds (id, module_id, wasm_sha256, build_manifest)
SELECT UUID(), id, wasm_sha256, build_manifest
FROM modules
WHERE wasm_sha256 IS NOT NULL;

ALTER TABLE modules ADD COLUMN current_build_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin;
ALTER TABLE modules
  ADD CONSTRAINT fk_modules_current_build
  FOREIGN KEY (current_build_id) REFERENCES module_builds (id) ON DELETE SET NULL;

UPDATE modules m
JOIN module_builds b ON b.module_id = m.id AND b.wasm_sha256 = m.wasm_sha256
SET m.current_build_id = b.id;

ALTER TABLE shop_modules ADD COLUMN build_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin;
ALTER TABLE shop_modules
  ADD CONSTRAINT fk_shop_modules_build
  FOREIGN KEY (module_id, build_id) REFERENCES module_builds (module_id, id);

UPDATE shop_modules sm
JOIN modules m ON m.id = sm.module_id
SET sm.build_id = m.current_build_id
WHERE m.current_build_id IS NOT NULL;
