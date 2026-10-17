package migrations

import "embed"

// Files contains the PostgreSQL schema migrations shipped with the application.
//
//go:embed *.sql
var Files embed.FS

// MariaDBFiles contains the MariaDB-specific schema migrations.
//
//go:embed mariadb/*.sql
var MariaDBFiles embed.FS
