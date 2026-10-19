# MariaDB support

MariaDB requires its own schema and SQL dialect; PostgreSQL migrations are not
portable by merely changing the connection driver. The MariaDB schema is in
[`migrations/mariadb/0001_mariadb_schema.sql`](../migrations/mariadb/0001_mariadb_schema.sql).
It creates the application tables, shipping configuration tables, migration
ledger, and `updated_at` triggers. It deliberately does not insert demo data.

## Initialize a database

The preferred path is the migration command, which detects the URL scheme and
uses the MariaDB migration set:

```powershell
$env:DATABASE_URL = "mariadb://USER:PASSWORD@HOST:3306/DATABASE?tls=true"
go run ./cmd/db-migrate
```

For an empty database this applies `0001_mariadb_schema.sql`, then the synthetic
catalog and shipping fixture in `0002_mariadb_demo_seed.sql`. If the schema was
already installed by phpMyAdmin, the command recognizes its migration ledger
and applies only the missing seed migration. Keep credentials out of shell
history and source control.

For manual schema installation, select a **new, empty** database in phpMyAdmin,
import `0001_mariadb_schema.sql`, and then run `go run ./cmd/db-migrate` to apply
the remaining migrations. The initial DDL is not an upgrade script and must not
be re-imported over an existing schema. MariaDB commits DDL statements
individually, so an interrupted baseline install may need manual cleanup before
it can be retried. The migrator refuses to replay the baseline if it finds
application tables without a migration ledger.

PostgreSQL continues to use the root-level migrations unchanged. The MariaDB
connection uses its own placeholder, cast, JSON, upsert, and row-lock handling;
the checkout, catalog administration, staff, and migration paths select the
MariaDB-compatible SQL behavior where syntax differs.

PostgreSQL remains the reference connector. Real integration tests now exercise
the MariaDB 11.8.8 driver, migration/commerce flow, and database backup/restore
on isolated local test databases. They do not qualify a production deployment
or the IONOS host. The IONOS phpMyAdmin session confirmed MariaDB 11.8.8 and
access to a database with 36 tables; it did not validate the Go driver or
migration runner there. Direct access from the development machine was blocked
because the database hostname did not resolve (`NXDOMAIN`).
Consult [the connector comparison](database-support.md) for the current
validation status.

The opt-in Go integration test applies the MariaDB migrations when needed and
exercises catalog reads/writes, category and module fixtures, shipping JSON
lookup, cart checkout, order reading, and cancellation:

```powershell
$env:MARIADB_TEST_DATABASE_URL = "mariadb://USER:PASSWORD@HOST:3306/DEDICATED_TEST_DATABASE?tls=true"
go test -tags integration ./internal/store -run TestMariaDBMigrationAndCommerceFlow
```

Use a dedicated, disposable test database: the test installs the schema and
synthetic fixtures when missing, then creates and removes temporary commerce
records. Do not point it at a database with user or production data.

## Backup and restore

`cmd/db-backup` supports MariaDB with `mariadb-dump` (or `mysqldump`) and the
`mariadb` (or `mysql`) restore client. Configure `DATABASE_URL`,
`EUROMNI_ARTIFACT_DIR`, and `EUROMNI_BACKUP_DIR` as private environment
variables, then run `go run ./cmd/db-backup`. MariaDB restoration uses
`-restore-database` and requires a newly created, empty database of the same
dialect; it does not create databases or restore server accounts and global
privileges. Restore the FileStore separately with `-restore-artifacts`.

The opt-in integration test exercises a real backup and restore, including
table data, triggers, and routines. It creates and drops a second database, so
the dedicated source test account must have create/drop database privileges:

```powershell
$env:MARIADB_BACKUP_TEST_DATABASE_URL = "mariadb://USER:PASSWORD@HOST:3306/DEDICATED_TEST_DATABASE?tls=true"
go test -tags integration -count=1 ./internal/backup -run TestBackupAndRestoreMariaDB
```

Never use a production database for this test. See the common
[backup and restore procedure](database-backup.md) for operational requirements,
credential handling, limitations, and PostgreSQL commands.
