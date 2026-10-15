//go:build integration

package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	migrationrunner "euromni/internal/migrations"
	schemafiles "euromni/migrations"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Start(t testing.TB) *sql.DB {
	t.Helper()
	repoRoot := projectRoot(t)
	scratchDir := filepath.Join(repoRoot, ".tmp", "embedded-postgres")
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		t.Fatalf("create local PostgreSQL scratch directory: %v", err)
	}
	instanceDir, err := os.MkdirTemp(scratchDir, "instance-")
	if err != nil {
		t.Fatalf("create local PostgreSQL instance directory: %v", err)
	}
	port, err := availablePort()
	if err != nil {
		_ = os.RemoveAll(instanceDir)
		t.Fatalf("allocate local PostgreSQL port: %v", err)
	}

	config := embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(port).
		Database("euromni_test").
		Username("euromni_test").
		Password("test-only-password").
		Locale("C").
		Encoding("UTF8").
		StartTimeout(2 * time.Minute).
		CachePath(filepath.Join(scratchDir, "cache")).
		BinariesPath(filepath.Join(instanceDir, "binaries")).
		RuntimePath(filepath.Join(instanceDir, "runtime")).
		DataPath(filepath.Join(instanceDir, "data")).
		Logger(io.Discard)
	postgres := embeddedpostgres.NewDatabase(config)
	if err := postgres.Start(); err != nil {
		_ = os.RemoveAll(instanceDir)
		t.Fatalf("start local PostgreSQL 16: %v", err)
	}

	var db *sql.DB
	t.Cleanup(func() {
		if db != nil {
			if err := db.Close(); err != nil {
				t.Errorf("close local PostgreSQL connection: %v", err)
			}
		}
		if err := postgres.Stop(); err != nil {
			t.Errorf("stop local PostgreSQL: %v", err)
		}
		if err := os.RemoveAll(instanceDir); err != nil {
			t.Errorf("remove local PostgreSQL test directory: %v", err)
		}
	})

	connectionURL := config.GetConnectionURL() + "?sslmode=disable&default_query_exec_mode=simple_protocol"
	db, err = sql.Open("pgx", connectionURL)
	if err != nil {
		t.Fatalf("open local PostgreSQL connection: %v", err)
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to local PostgreSQL: %v", err)
	}

	if _, err := migrationrunner.Apply(ctx, db, schemafiles.Files); err != nil {
		t.Fatalf("apply local PostgreSQL migrations: %v", err)
	}
	db.SetMaxOpenConns(10)
	return db
}

func projectRoot(t testing.TB) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration fixture source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	for _, path := range []string{"go.mod", filepath.Join("migrations", "0001_init.sql")} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("project root %s does not contain %s: %v", root, path, err)
		}
	}
	return root
}

func availablePort() (uint32, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return uint32(address.Port), nil
}
