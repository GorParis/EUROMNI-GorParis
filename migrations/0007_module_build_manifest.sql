BEGIN;

ALTER TABLE modules
  ADD COLUMN build_manifest jsonb,
  ADD CONSTRAINT modules_build_manifest_object_check
    CHECK (build_manifest IS NULL OR jsonb_typeof(build_manifest) = 'object');

INSERT INTO schema_migrations (version) VALUES ('0007_module_build_manifest.sql');

COMMIT;