package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"euromni/internal/artifact"
	"github.com/jackc/pgx/v5"
)

func TestCreateAndVerifyBackup(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifacts")
	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	const artifactContents = "verified wasm artifact"
	createdArtifact, err := artifactStore.Put(context.Background(), strings.NewReader(artifactContents))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 17, 10, 0, 0, 0, time.UTC)
	result, err := create(context.Background(), Options{
		DatabaseURL: "postgres://user:secret@localhost/euromni?sslmode=disable",
		ArtifactDir: artifactRoot,
		BackupDir:   filepath.Join(root, "backups"),
		Now:         now,
	}, func(_ context.Context, _, destination string) error {
		return os.WriteFile(destination, []byte("database snapshot"), 0o600)
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Manifest.ArtifactCount != 1 || result.Manifest.ArtifactBytes != int64(len(artifactContents)) {
		t.Fatalf("artifact metadata = count %d bytes %d", result.Manifest.ArtifactCount, result.Manifest.ArtifactBytes)
	}
	if result.Manifest.DatabaseDialect != "postgres" {
		t.Fatalf("database dialect = %q, want postgres", result.Manifest.DatabaseDialect)
	}
	verified, err := Verify(result.Directory)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified != result.Manifest {
		t.Fatalf("verified manifest = %+v, want %+v", verified, result.Manifest)
	}
	checkArchivedArtifact(t, filepath.Join(result.Directory, result.Manifest.ArtifactsFile), createdArtifact.SHA256, artifactContents)

	restoredRoot := filepath.Join(root, "restored-artifacts")
	if _, err := RestoreArtifacts(context.Background(), result.Directory, restoredRoot); err != nil {
		t.Fatalf("RestoreArtifacts() error = %v", err)
	}
	restoredStore, err := artifact.NewFileStore(restoredRoot)
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
}

func TestCreateRejectsUnsafeOrMissingConfiguration(t *testing.T) {
	root := t.TempDir()
	base := Options{
		DatabaseURL: "postgres://user:secret@localhost/euromni",
		ArtifactDir: filepath.Join(root, "artifacts"),
		BackupDir:   filepath.Join(root, "backups"),
	}
	for name, options := range map[string]Options{
		"missing database":      {ArtifactDir: base.ArtifactDir, BackupDir: base.BackupDir},
		"missing artifact root": {DatabaseURL: base.DatabaseURL, BackupDir: base.BackupDir},
		"missing backup root":   {DatabaseURL: base.DatabaseURL, ArtifactDir: base.ArtifactDir},
		"overlapping roots":     {DatabaseURL: base.DatabaseURL, ArtifactDir: base.ArtifactDir, BackupDir: filepath.Join(base.ArtifactDir, "nested")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := create(context.Background(), options, func(context.Context, string, string) error {
				t.Fatal("database dump command was called for invalid backup options")
				return nil
			})
			if err == nil {
				t.Fatal("Create() unexpectedly succeeded")
			}
		})
	}
}

func TestCreateFailureRemovesPartialBackup(t *testing.T) {
	root := t.TempDir()
	backupRoot := filepath.Join(root, "backups")
	_, err := create(context.Background(), Options{
		DatabaseURL: "postgres://user:secret@localhost/euromni",
		ArtifactDir: filepath.Join(root, "artifacts"),
		BackupDir:   backupRoot,
	}, func(_ context.Context, _, destination string) error {
		if err := os.WriteFile(destination, []byte("partial"), 0o600); err != nil {
			return err
		}
		return errors.New("database dump failed")
	})
	if err == nil {
		t.Fatal("Create() unexpectedly succeeded")
	}
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial backup staging entries remain: %v", entries)
	}
}

func TestRestoreDatabaseRejectsDifferentDialectAndSupportsLegacyManifest(t *testing.T) {
	root := t.TempDir()
	result, err := create(context.Background(), Options{
		DatabaseURL: "postgres://user:pass@localhost/euromni",
		ArtifactDir: filepath.Join(root, "artifacts"),
		BackupDir:   filepath.Join(root, "backups"),
	}, func(_ context.Context, _, destination string) error {
		return os.WriteFile(destination, []byte("test dump"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyManifest := result.Manifest
	legacyManifest.DatabaseDialect = ""
	manifestPath := filepath.Join(result.Directory, manifestName)
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(manifestPath, legacyManifest); err != nil {
		t.Fatal(err)
	}
	_, err = RestoreDatabase(context.Background(), result.Directory, "mariadb://user:pass@localhost/euromni")
	if err == nil || !strings.Contains(err.Error(), `backup database dialect "postgres" does not match`) {
		t.Fatalf("RestoreDatabase() error = %v, want legacy PostgreSQL dialect mismatch", err)
	}
}

func TestVerifyRejectsModifiedBackup(t *testing.T) {
	root := t.TempDir()
	result, err := create(context.Background(), Options{
		DatabaseURL: "postgres://user:secret@localhost/euromni",
		ArtifactDir: filepath.Join(root, "artifacts"),
		BackupDir:   filepath.Join(root, "backups"),
	}, func(_ context.Context, _, destination string) error {
		return os.WriteFile(destination, []byte("database snapshot"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Directory, result.Manifest.DatabaseFile), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(result.Directory); err == nil {
		t.Fatal("Verify() accepted a modified database dump")
	}
}

func TestPruneRetainsDailyWeeklyAndMonthlyBackups(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "artifacts")
	backupRoot := filepath.Join(root, "backups")
	now := time.Date(2026, time.October, 17, 10, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -100)
	var names []string
	for at := start; !at.After(now); at = at.AddDate(0, 0, 1) {
		result, err := create(context.Background(), Options{
			DatabaseURL: "postgres://user:secret@localhost/euromni",
			ArtifactDir: artifactRoot,
			BackupDir:   backupRoot,
			Now:         at,
		}, func(_ context.Context, _, destination string) error {
			return os.WriteFile(destination, []byte("database snapshot"), 0o600)
		})
		if err != nil {
			t.Fatalf("Create(%s) error = %v", at, err)
		}
		names = append(names, filepath.Base(result.Directory))
	}
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 34 {
		t.Fatalf("retained %d backups, want at most 34", len(entries))
	}
	for _, name := range names[len(names)-14:] {
		if _, err := os.Stat(filepath.Join(backupRoot, name)); err != nil {
			t.Fatalf("recent daily backup %s was pruned: %v", name, err)
		}
	}
	weekly := map[string]bool{}
	monthly := map[string]bool{}
	weekStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekday := int(weekStart.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weeklyCutoff := weekStart.AddDate(0, 0, -(weekday-1)).AddDate(0, 0, -7*7)
	for _, entry := range entries {
		manifest, err := readManifest(filepath.Join(backupRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !manifest.CreatedAt.Before(weeklyCutoff) {
			year, week := manifest.CreatedAt.UTC().ISOWeek()
			weekly[fmt.Sprintf("%04d-%02d", year, week)] = true
		}
		monthly[manifest.CreatedAt.UTC().Format("2006-01")] = true
	}
	if len(weekly) != 8 {
		t.Fatalf("retained backups cover %d recent weeks, want 8", len(weekly))
	}
	if len(monthly) > 12 {
		t.Fatalf("retained backups span %d months, want at most 12", len(monthly))
	}
}

func TestRetentionKeeps12MonthlyBackups(t *testing.T) {
	now := time.Date(2026, time.October, 17, 10, 0, 0, 0, time.UTC)
	var backups []backupEntry
	for at := now.AddDate(0, -15, 0); !at.After(now); at = at.AddDate(0, 0, 1) {
		backups = append(backups, backupEntry{
			name:      "euromni-backup-" + at.Format("20060102T150405Z") + "-00000000",
			createdAt: at,
		})
	}
	retained := retainedBackups(backups, now)
	monthlyCutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
	months := map[string]bool{}
	for _, entry := range backups {
		if _, exists := retained[entry.name]; exists && !entry.createdAt.Before(monthlyCutoff) {
			months[entry.createdAt.Format("2006-01")] = true
		}
	}
	if len(months) != 12 {
		t.Fatalf("retained %d monthly backups, want 12", len(months))
	}
}

func TestPGDumpEnvironmentDoesNotExposeDatabaseURL(t *testing.T) {
	config, err := pgx.ParseConfig("postgres://backup:top-secret@db.example.test:5433/euromni?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	environment, err := pgDumpEnvironment(config, "postgres://backup:top-secret@db.example.test:5433/euromni?sslmode=disable&sslrootcert=%2Ftmp%2Froot.crt")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, variable := range environment {
		name, value, _ := strings.Cut(variable, "=")
		values[name] = value
	}
	if values["PGPASSWORD"] != "top-secret" || values["PGSSLMODE"] != "disable" ||
		values["PGSSLROOTCERT"] != "/tmp/root.crt" {
		t.Fatalf("pg_dump environment omitted connection settings: %#v", values)
	}
	for _, variable := range environment {
		if strings.HasPrefix(variable, "DATABASE_URL=") {
			t.Fatal("DATABASE_URL was passed to pg_dump environment")
		}
	}
}

func TestMariaDBClientConfigAndPrivateOptions(t *testing.T) {
	config, err := mariaDBClientConfig("mariadb://user%22name:p%5C%22ass@db.example:3307/euromni?tls=true")
	if err != nil {
		t.Fatal(err)
	}
	if config.User != `user"name` || config.Passwd != `p\"ass` || config.Addr != "db.example:3307" ||
		config.DBName != "euromni" || config.TLSConfig != "true" {
		t.Fatalf("MariaDB client config = %#v", config)
	}
	path, cleanup, err := writeMariaDBClientOptions(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("remove MariaDB client options: %v", err)
		}
	}()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("MariaDB client options permissions = %o; want owner-only", info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`user="user\"name"`,
		`password="p\\\"ass"`,
		"ssl=true",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("MariaDB client options omitted %q: %s", want, content)
		}
	}
}

func TestMariaDBClientConfigRejectsCustomTLSAndPostgreSQL(t *testing.T) {
	for _, databaseURL := range []string{
		"mariadb://user:pass@db.example/euromni?tls=custom",
		"postgres://user:pass@db.example/euromni",
	} {
		if _, err := mariaDBClientConfig(databaseURL); err == nil {
			t.Errorf("mariaDBClientConfig(%q) unexpectedly succeeded", databaseURL)
		}
	}
}

func checkArchivedArtifact(t *testing.T, archive, digest, want string) {
	t.Helper()
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != filepath.ToSlash(filepath.Join(digest[:2], digest)) {
			t.Fatalf("archive entry = %q, want artifact digest path", header.Name)
		}
		contents, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != want {
			t.Fatalf("artifact archive contents = %q, want %q", contents, want)
		}
		return
	}
	t.Fatal("artifact was not included in archive")
}
