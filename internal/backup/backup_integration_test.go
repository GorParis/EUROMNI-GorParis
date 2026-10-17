//go:build integration

package backup

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"euromni/internal/artifact"
	"euromni/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestBackupAndRestorePostgreSQL(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(testFilePath(t)), "..", ".."))
		toolName := "pg_dump"
		if runtime.GOOS == "windows" {
			toolName += ".exe"
		}
		toolDirectory := filepath.Join(repositoryRoot, ".tmp", "postgresql-16-tools", "pgsql", "bin")
		if _, err := os.Stat(filepath.Join(toolDirectory, toolName)); err != nil {
			t.Skipf("pg_dump is unavailable in PATH and local PostgreSQL tools: %v", err)
		}
		t.Setenv("PATH", toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skipf("pg_restore is unavailable: %v", err)
	}

	database := testdb.Start(t)
	var port int
	var databaseName string
	if err := database.QueryRow("SELECT current_setting('port')::integer, current_database()").Scan(&port, &databaseName); err != nil {
		t.Fatal(err)
	}
	databaseURL := fmt.Sprintf("postgres://euromni_test:test-only-password@127.0.0.1:%d/%s?sslmode=disable", port, databaseName)
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifacts")
	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	const artifactContents = "artifact included in database backup"
	createdArtifact, err := artifactStore.Put(context.Background(), strings.NewReader(artifactContents))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Create(context.Background(), Options{
		DatabaseURL: databaseURL,
		ArtifactDir: artifactRoot,
		BackupDir:   filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := Verify(result.Directory); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	restoredArtifacts := filepath.Join(root, "restored-artifacts")
	if _, err := RestoreArtifacts(context.Background(), result.Directory, restoredArtifacts); err != nil {
		t.Fatalf("RestoreArtifacts() error = %v", err)
	}
	restoredStore, err := artifact.NewFileStore(restoredArtifacts)
	if err != nil {
		t.Fatal(err)
	}
	restoredContents, err := restoredStore.Get(context.Background(), createdArtifact.SHA256)
	if err != nil {
		t.Fatalf("read restored artifact: %v", err)
	}
	if string(restoredContents) != artifactContents {
		t.Fatalf("restored artifact contents = %q, want %q", restoredContents, artifactContents)
	}

	restoreDatabase := fmt.Sprintf("euromni_restore_%d", time.Now().UnixNano())
	admin, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{restoreDatabase}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{restoreDatabase}.Sanitize()); err != nil {
			t.Errorf("drop restored test database: %v", err)
		}
	}()

	environment, err := pgDumpEnvironment(config, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("pg_restore",
		"--no-owner",
		"--no-acl",
		"--host", config.Host,
		"--port", strconv.Itoa(int(config.Port)),
		"--username", config.User,
		"--dbname", restoreDatabase,
		filepath.Join(result.Directory, result.Manifest.DatabaseFile),
	)
	command.Env = environment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("pg_restore failed: %v: %s", err, strings.TrimSpace(string(output)))
	}

	restoreURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	restoreURL.Path = "/" + restoreDatabase
	restored, err := pgx.Connect(context.Background(), restoreURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close(context.Background())
	var migrations int
	if err := restored.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations == 0 {
		t.Fatal("restored database contains no migration records")
	}
	checkArchivedArtifact(
		t,
		filepath.Join(result.Directory, result.Manifest.ArtifactsFile),
		createdArtifact.SHA256,
		artifactContents,
	)
}

func testFilePath(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate backup integration test source")
	}
	return source
}
