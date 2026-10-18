package dbdialect

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestResolvePostgreSQLURL(t *testing.T) {
	driver, dsn, dialect, err := Resolve("postgres://user:pass@localhost:5432/euromni")
	if err != nil {
		t.Fatal(err)
	}
	if driver != "pgx" || dsn != "postgres://user:pass@localhost:5432/euromni" || dialect != PostgreSQL {
		t.Fatalf("unexpected PostgreSQL resolution: driver=%q dsn=%q dialect=%q", driver, dsn, dialect)
	}
}

func TestResolveMariaDBURL(t *testing.T) {
	driver, dsn, dialect, err := Resolve("mariadb://user:p%40ss@db.example:3306/shop?charset=utf8mb4&loc=UTC")
	if err != nil {
		t.Fatal(err)
	}
	if driver != mariaDBDriverName || dialect != MariaDB {
		t.Fatalf("unexpected MariaDB resolution: driver=%q dialect=%q", driver, dialect)
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse generated driver DSN: %v", err)
	}
	if config.User != "user" || config.Passwd != "p@ss" || config.Addr != "db.example:3306" || config.DBName != "shop" {
		t.Fatalf("unexpected MariaDB connection settings: %#v", config)
	}
	if !config.ParseTime || !config.MultiStatements {
		t.Fatalf("MariaDB timestamps and multi-statement migrations must be enabled")
	}
}

func TestResolveMariaDBDriverDSN(t *testing.T) {
	driver, dsn, dialect, err := Resolve("user:pass@tcp(db.example:3306)/shop?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	if driver != mariaDBDriverName || dialect != MariaDB {
		t.Fatalf("unexpected MariaDB DSN resolution: driver=%q dsn=%q dialect=%q", driver, dsn, dialect)
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse generated driver DSN: %v", err)
	}
	if config.User != "user" || config.Passwd != "pass" || config.Addr != "db.example:3306" || config.DBName != "shop" {
		t.Fatalf("unexpected MariaDB connection settings: %#v", config)
	}
	if !config.ParseTime || !config.MultiStatements {
		t.Fatal("raw MariaDB DSNs must enable time parsing and migration scripts")
	}
}

func TestResolveRejectsUnsupportedMariaDBURLOptions(t *testing.T) {
	if _, _, _, err := Resolve("mariadb://user:pass@db.example/shop?ssl-mode=required"); err == nil {
		t.Fatal("expected unsupported URL option to be rejected")
	}
}

func TestResolveMariaDBURLEnablesTLSAndUTC(t *testing.T) {
	driver, dsn, dialect, err := Resolve("mariadb://db.example:3306/shop?tls=true")
	if err != nil {
		t.Fatal(err)
	}
	if driver != mariaDBDriverName || dialect != MariaDB {
		t.Fatalf("unexpected MariaDB resolution: driver=%q dialect=%q", driver, dialect)
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.TLSConfig != "true" || config.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("unexpected TLS/time-zone settings: %#v", config)
	}
}

func TestResolveMariaDBURLDefaultsPort(t *testing.T) {
	driver, dsn, dialect, err := Resolve("mariadb://db.example/shop")
	if err != nil {
		t.Fatal(err)
	}
	if driver != mariaDBDriverName || dialect != MariaDB {
		t.Fatalf("unexpected MariaDB resolution: driver=%q dialect=%q", driver, dialect)
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.Addr != "db.example:3306" {
		t.Fatalf("MariaDB URL without a port resolved to %q", config.Addr)
	}
}

func TestIsUniqueConstraintForBothDialects(t *testing.T) {
	if !IsUniqueConstraint(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("PostgreSQL unique violation was not recognized")
	}
	if !IsUniqueConstraint(&mysql.MySQLError{Number: 1062}) {
		t.Fatal("MariaDB duplicate-key error was not recognized")
	}
	if IsUniqueConstraint(errors.New("unrelated error")) {
		t.Fatal("unrelated database error was classified as a unique violation")
	}
}
