//go:build integration

package backup

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"euromni/internal/artifact"
	"euromni/internal/dbdialect"
	"euromni/internal/migrations"
	"euromni/internal/testdb"
	schemafiles "euromni/migrations"
	"github.com/go-sql-driver/mysql"
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

	restoreURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	restoreURL.Path = "/" + restoreDatabase
	targetAdmin, err := pgx.Connect(context.Background(), restoreURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer targetAdmin.Close(context.Background())
	if _, err := targetAdmin.Exec(context.Background(),
		"CREATE FUNCTION euromni_restore_blocker() RETURNS integer LANGUAGE sql AS 'SELECT 1'"); err != nil {
		t.Fatalf("create PostgreSQL restore blocker: %v", err)
	}
	if _, err := RestoreDatabase(context.Background(), result.Directory, restoreURL.String()); err == nil ||
		!strings.Contains(err.Error(), "destination database is not empty") {
		t.Fatalf("RestoreDatabase() on non-empty PostgreSQL returned %v, want non-empty destination error", err)
	}
	if _, err := targetAdmin.Exec(context.Background(), "DROP FUNCTION euromni_restore_blocker()"); err != nil {
		t.Fatalf("drop PostgreSQL restore blocker: %v", err)
	}
	if err := targetAdmin.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreDatabase(context.Background(), result.Directory, restoreURL.String()); err != nil {
		t.Fatalf("RestoreDatabase() error = %v", err)
	}
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

func TestBackupAndRestoreMariaDB(t *testing.T) {
	databaseURL := os.Getenv("MARIADB_BACKUP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MARIADB_BACKUP_TEST_DATABASE_URL must point to a dedicated MariaDB test database")
	}
	_, err := exec.LookPath("mariadb-dump")
	if err != nil {
		_, err = exec.LookPath("mysqldump")
		if err != nil {
			t.Skip("mariadb-dump or mysqldump is unavailable")
		}
	}
	_, err = exec.LookPath("mariadb")
	if err != nil {
		_, err = exec.LookPath("mysql")
		if err != nil {
			t.Skip("mariadb or mysql client is unavailable")
		}
	}

	driverName, driverDSN, dialect, err := dbdialect.Resolve(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if dialect != dbdialect.MariaDB {
		t.Fatalf("MARIADB_BACKUP_TEST_DATABASE_URL resolved to %q, want MariaDB", dialect)
	}
	database, err := sql.Open(driverName, driverDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	mariaDBMigrations, err := fs.Sub(schemafiles.MariaDBFiles, "mariadb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrations.ApplyMariaDB(ctx, database, mariaDBMigrations); err != nil {
		t.Fatalf("apply MariaDB backup integration migrations: %v", err)
	}
	probeTable := fmt.Sprintf("euromni_backup_probe_%d", time.Now().UnixNano())
	if _, err := database.ExecContext(ctx, "CREATE TABLE "+probeTable+
		" (id INT PRIMARY KEY, payload VARCHAR(255) NOT NULL) ENGINE=InnoDB"); err != nil {
		t.Fatalf("create MariaDB backup probe table: %v", err)
	}
	defer func() {
		if _, err := database.ExecContext(context.Background(), "DROP TABLE "+probeTable); err != nil {
			t.Errorf("drop MariaDB backup probe table: %v", err)
		}
	}()
	triggerName := probeTable + "_trigger"
	if _, err := database.ExecContext(ctx, "CREATE TRIGGER "+triggerName+" BEFORE INSERT ON "+probeTable+
		" FOR EACH ROW SET NEW.payload = CONCAT(NEW.payload, '|trigger')"); err != nil {
		t.Fatalf("create MariaDB backup probe trigger: %v", err)
	}
	procedureName := probeTable + "_procedure"
	if _, err := database.ExecContext(ctx, "CREATE PROCEDURE "+procedureName+
		"() SELECT 'MariaDB routine restored'"); err != nil {
		t.Fatalf("create MariaDB backup probe procedure: %v", err)
	}
	defer func() {
		if _, err := database.ExecContext(context.Background(), "DROP PROCEDURE "+procedureName); err != nil {
			t.Errorf("drop MariaDB backup probe procedure: %v", err)
		}
	}()
	const probeValue = "MariaDB backup and restore verified"
	if _, err := database.ExecContext(ctx, "INSERT INTO "+probeTable+
		" (id, payload) VALUES (1, '"+probeValue+"')"); err != nil {
		t.Fatalf("insert MariaDB backup probe: %v", err)
	}

	parsedURL, err := url.Parse(databaseURL)
	if err != nil || parsedURL.Scheme == "" {
		t.Fatalf("MARIADB_BACKUP_TEST_DATABASE_URL must be a MariaDB URL: %v", err)
	}
	restoreDatabaseName := fmt.Sprintf("euromni_restore_%d", time.Now().UnixNano())
	adminConfig, err := mysql.ParseDSN(driverDSN)
	if err != nil {
		t.Fatal(err)
	}
	adminConfig.DBName = ""
	admin, err := sql.Open(driverName, adminConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	quotedRestoreDatabase := "`" + strings.ReplaceAll(restoreDatabaseName, "`", "``") + "`"
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quotedRestoreDatabase); err != nil {
		t.Fatalf("create MariaDB restore test database: %v", err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+quotedRestoreDatabase); err != nil {
			t.Errorf("drop MariaDB restore test database: %v", err)
		}
	}()
	restoreURL := *parsedURL
	restoreURL.Path = "/" + restoreDatabaseName

	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifacts")
	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	const artifactContents = "MariaDB backup includes FileStore"
	createdArtifact, err := artifactStore.Put(ctx, strings.NewReader(artifactContents))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Create(ctx, Options{
		DatabaseURL: databaseURL,
		ArtifactDir: artifactRoot,
		BackupDir:   filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatalf("Create() MariaDB backup error = %v", err)
	}
	if result.Manifest.DatabaseDialect != string(dbdialect.MariaDB) {
		t.Fatalf("backup database dialect = %q, want MariaDB", result.Manifest.DatabaseDialect)
	}
	if _, err := admin.ExecContext(ctx, "CREATE TABLE "+quotedRestoreDatabase+".restore_blocker (id INT)"); err != nil {
		t.Fatalf("create MariaDB restore blocker: %v", err)
	}
	if _, err := RestoreDatabase(ctx, result.Directory, restoreURL.String()); err == nil ||
		!strings.Contains(err.Error(), "destination database is not empty") {
		t.Fatalf("RestoreDatabase() on non-empty MariaDB returned %v, want non-empty destination error", err)
	}
	if _, err := admin.ExecContext(ctx, "DROP TABLE "+quotedRestoreDatabase+".restore_blocker"); err != nil {
		t.Fatalf("drop MariaDB restore blocker: %v", err)
	}
	if _, err := RestoreDatabase(ctx, result.Directory, restoreURL.String()); err != nil {
		t.Fatalf("RestoreDatabase() MariaDB error = %v", err)
	}
	restoreDriver, restoreDSN, _, err := dbdialect.Resolve(restoreURL.String())
	if err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open(restoreDriver, restoreDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var restoredValue string
	if err := restored.QueryRowContext(ctx,
		"SELECT payload FROM "+probeTable+" WHERE id = 1").Scan(&restoredValue); err != nil {
		t.Fatalf("read restored MariaDB probe: %v", err)
	}
	if restoredValue != probeValue+"|trigger" {
		t.Fatalf("restored MariaDB probe = %q, want %q", restoredValue, probeValue+"|trigger")
	}
	if _, err := restored.ExecContext(ctx, "INSERT INTO "+probeTable+
		" (id, payload) VALUES (2, 'trigger check')"); err != nil {
		t.Fatalf("execute restored MariaDB trigger: %v", err)
	}
	if err := restored.QueryRowContext(ctx,
		"SELECT payload FROM "+probeTable+" WHERE id = 2").Scan(&restoredValue); err != nil {
		t.Fatalf("read restored MariaDB trigger result: %v", err)
	}
	if restoredValue != "trigger check|trigger" {
		t.Fatalf("restored MariaDB trigger result = %q, want trigger transformation", restoredValue)
	}
	var restoredRoutine string
	if err := restored.QueryRowContext(ctx, "CALL "+procedureName+"()").Scan(&restoredRoutine); err != nil {
		t.Fatalf("call restored MariaDB procedure: %v", err)
	}
	if restoredRoutine != "MariaDB routine restored" {
		t.Fatalf("restored MariaDB procedure returned %q", restoredRoutine)
	}
	var restoredProducts int
	if err := restored.QueryRowContext(ctx, "SELECT count(*) FROM products").Scan(&restoredProducts); err != nil {
		t.Fatalf("read restored MariaDB product catalog: %v", err)
	}
	if restoredProducts == 0 {
		t.Fatal("restored MariaDB database contains no seeded products")
	}
	restoredArtifactPath := filepath.Join(root, "restored-artifacts")
	if _, err := RestoreArtifacts(ctx, result.Directory, restoredArtifactPath); err != nil {
		t.Fatalf("RestoreArtifacts() MariaDB backup error = %v", err)
	}
	restoredArtifactStore, err := artifact.NewFileStore(restoredArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	restoredArtifactContents, err := restoredArtifactStore.Get(ctx, createdArtifact.SHA256)
	if err != nil {
		t.Fatalf("read restored MariaDB FileStore artifact: %v", err)
	}
	if string(restoredArtifactContents) != artifactContents {
		t.Fatalf("restored MariaDB FileStore artifact = %q, want %q", restoredArtifactContents, artifactContents)
	}
}

func testFilePath(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate backup integration test source")
	}
	return source
}
