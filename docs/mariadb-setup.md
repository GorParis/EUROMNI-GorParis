# MariaDB support

MariaDB requires its own schema and SQL dialect; PostgreSQL migrations are not
portable by merely changing the connection driver. The MariaDB schema is in
[`migrations/mariadb/0001_mariadb_schema.sql`](../migrations/mariadb/0001_mariadb_schema.sql).
It creates the application tables, shipping configuration tables, migration
ledger, and `updated_at` triggers. It deliberately does not insert demo data.

## Import into a new database

This is a one-time schema import for a **new, empty MariaDB database**:

1. In phpMyAdmin, select the target database and confirm it contains no tables.
2. Open **Import**, choose `0001_mariadb_schema.sql`, leave the format as SQL,
   and run the import.
3. Verify that the database has 36 base tables and that
   `schema_migrations` contains `0001_mariadb_schema.sql`.

The script is not an upgrade script and must not be re-imported over an existing
schema. PostgreSQL keeps using the root-level migrations unchanged.

The MariaDB schema was imported into the provided empty MariaDB 11.8 database.
The first import reported a reserved-word syntax error in the `module_kv` primary
key; the key was quoted and that missing table was created separately. The
result was verified at 36 base tables, one migration-ledger row, and no shop or
shipping data. A transactional smoke test successfully inserted a shop,
carrier, zone, and rate, checked JSON and the `updated_at` trigger, and rolled
back; follow-up counts confirmed that no smoke-test rows remained.

This proves the schema and basic MariaDB DDL/DML on that server, **not** full CMS
compatibility. The application queries and migration command still contain
PostgreSQL-specific behavior; do not point the running CMS at this database
until that work is completed and tested.
