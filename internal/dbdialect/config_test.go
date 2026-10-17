package dbdialect

import (
	"testing"

	"github.com/go-sql-driver/mysql"
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
	if driver != "mysql" || dialect != MariaDB {
		t.Fatalf("unexpected MariaDB resolution: driver=%q dialect=%q", driver, dialect)
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse generated driver DSN: %v", err)
	}
	if config.User != "user" || config.Passwd != "p@ss" || config.Addr != "db.example:3306" || config.DBName != "shop" {
		t.Fatalf("unexpected MariaDB connection settings: %#v", config)
	}
	if !config.ParseTime {
		t.Fatalf("MariaDB timestamps must be parsed as time values")
	}
}

func TestResolveMariaDBDriverDSN(t *testing.T) {
	driver, dsn, dialect, err := Resolve("user:pass@tcp(db.example:3306)/shop?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	if driver != "mysql" || dsn != "user:pass@tcp(db.example:3306)/shop?parseTime=true" || dialect != MariaDB {
		t.Fatalf("unexpected MariaDB DSN resolution: driver=%q dsn=%q dialect=%q", driver, dsn, dialect)
	}
}

func TestResolveRejectsUnsupportedMariaDBURLOptions(t *testing.T) {
	if _, _, _, err := Resolve("mariadb://user:pass@db.example/shop?tls=true"); err == nil {
		t.Fatal("expected unsupported URL option to be rejected")
	}
}
