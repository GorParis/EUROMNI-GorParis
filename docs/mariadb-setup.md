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

PostgreSQL remains the reference connector. MariaDB unit tests and compilation
of the integration test validate the adapter's local behavior, but do not
qualify a real MariaDB server. The IONOS phpMyAdmin session confirmed MariaDB
11.8.8 and access to a database with 36 tables; it did not validate the Go
driver or this migration runner. Direct access from the development machine
was blocked because the database hostname did not resolve (`NXDOMAIN`).
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
