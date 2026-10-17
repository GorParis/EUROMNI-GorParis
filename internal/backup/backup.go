package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"euromni/internal/artifact"
	"euromni/internal/privatefs"
	"github.com/jackc/pgx/v5"
)

const (
	manifestName  = "manifest.json"
	formatVersion = 1
	backupPrefix  = "euromni-backup-"
)

var (
	digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	backupPattern = regexp.MustCompile(`^euromni-backup-(\d{8}T\d{6}Z)-[a-f0-9]{8}$`)
)

type Options struct {
	DatabaseURL string
	ArtifactDir string
	BackupDir   string
	Now         time.Time
}

type Manifest struct {
	FormatVersion   int       `json:"format_version"`
	CreatedAt       time.Time `json:"created_at"`
	DatabaseFile    string    `json:"database_file"`
	DatabaseSHA256  string    `json:"database_sha256"`
	DatabaseBytes   int64     `json:"database_bytes"`
	ArtifactsFile   string    `json:"artifacts_file"`
	ArtifactsSHA256 string    `json:"artifacts_sha256"`
	ArtifactsBytes  int64     `json:"artifacts_bytes"`
	ArtifactCount   int       `json:"artifact_count"`
	ArtifactBytes   int64     `json:"artifact_bytes"`
}

type Result struct {
	Directory string
	Manifest  Manifest
	Pruned    []string
}

type dumpRunner func(context.Context, string, string) error

func Create(ctx context.Context, options Options) (Result, error) {
	return create(ctx, options, runPGDump)
}

func create(ctx context.Context, options Options, dump dumpRunner) (result Result, retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dump == nil {
		return Result{}, errors.New("pg_dump runner is required")
	}
	if strings.TrimSpace(options.DatabaseURL) == "" {
		return Result{}, errors.New("DATABASE_URL is required")
	}
	if strings.TrimSpace(options.ArtifactDir) == "" {
		return Result{}, errors.New("EUROMNI_ARTIFACT_DIR is required")
	}
	if strings.TrimSpace(options.BackupDir) == "" {
		return Result{}, errors.New("EUROMNI_BACKUP_DIR is required")
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	options.Now = options.Now.UTC().Truncate(time.Second)

	artifactRoot, err := filepath.Abs(options.ArtifactDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve artifact directory: %w", err)
	}
	backupRoot, err := filepath.Abs(options.BackupDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve backup directory: %w", err)
	}
	if pathsOverlap(artifactRoot, backupRoot) {
		return Result{}, errors.New("backup directory must be separate from the artifact directory")
	}
	if err := privatefs.EnsureDirectory(backupRoot); err != nil {
		return Result{}, fmt.Errorf("ensure private backup directory: %w", err)
	}
	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		return Result{}, fmt.Errorf("open private artifact store: %w", err)
	}

	token := make([]byte, 4)
	if _, err := rand.Read(token); err != nil {
		return Result{}, fmt.Errorf("generate backup identifier: %w", err)
	}
	name := backupPrefix + options.Now.Format("20060102T150405Z") + "-" + hex.EncodeToString(token)
	staging := filepath.Join(backupRoot, "."+name+".partial")
	if err := privatefs.EnsureDirectory(staging); err != nil {
		return Result{}, fmt.Errorf("create private backup staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			if cleanupErr := os.RemoveAll(staging); cleanupErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("remove incomplete backup: %w", cleanupErr))
			}
		}
	}()

	databasePath := filepath.Join(staging, "database.dump")
	if err := dump(ctx, options.DatabaseURL, databasePath); err != nil {
		return Result{}, err
	}
	databaseHash, databaseSize, err := hashFile(databasePath)
	if err != nil {
		return Result{}, fmt.Errorf("verify database dump: %w", err)
	}
	if databaseSize == 0 {
		return Result{}, errors.New("pg_dump produced an empty database dump")
	}

	artifactPath := filepath.Join(staging, "artifacts.tar.gz")
	artifactCount, artifactBytes, err := archiveArtifacts(ctx, artifactStore, artifactRoot, artifactPath)
	if err != nil {
		return Result{}, fmt.Errorf("archive FileStore: %w", err)
	}
	artifactHash, archiveSize, err := hashFile(artifactPath)
	if err != nil {
		return Result{}, fmt.Errorf("verify artifact archive: %w", err)
	}

	manifest := Manifest{
		FormatVersion:   formatVersion,
		CreatedAt:       options.Now,
		DatabaseFile:    "database.dump",
		DatabaseSHA256:  databaseHash,
		DatabaseBytes:   databaseSize,
		ArtifactsFile:   "artifacts.tar.gz",
		ArtifactsSHA256: artifactHash,
		ArtifactsBytes:  archiveSize,
		ArtifactCount:   artifactCount,
		ArtifactBytes:   artifactBytes,
	}
	if err := writeManifest(filepath.Join(staging, manifestName), manifest); err != nil {
		return Result{}, err
	}
	finalPath := filepath.Join(backupRoot, name)
	if _, err := os.Lstat(finalPath); err == nil {
		return Result{}, fmt.Errorf("backup destination already exists: %s", finalPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect backup destination: %w", err)
	}
	if err := os.Rename(staging, finalPath); err != nil {
		return Result{}, fmt.Errorf("publish backup: %w", err)
	}
	published = true

	pruned, err := Prune(backupRoot, options.Now)
	if err != nil {
		return Result{Directory: finalPath, Manifest: manifest}, fmt.Errorf("backup created at %s but retention failed: %w", finalPath, err)
	}
	return Result{Directory: finalPath, Manifest: manifest, Pruned: pruned}, nil
}

func Verify(directory string) (Manifest, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve backup directory: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return Manifest{}, fmt.Errorf("inspect backup directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, errors.New("backup path must be a directory and not a symlink")
	}
	manifest, err := readManifest(absolute)
	if err != nil {
		return Manifest{}, err
	}
	for _, file := range []struct {
		name   string
		digest string
		size   int64
	}{
		{manifest.DatabaseFile, manifest.DatabaseSHA256, manifest.DatabaseBytes},
		{manifest.ArtifactsFile, manifest.ArtifactsSHA256, manifest.ArtifactsBytes},
	} {
		if filepath.Base(file.name) != file.name || file.name == "." || file.name == ".." {
			return Manifest{}, fmt.Errorf("invalid backup file name %q", file.name)
		}
		path := filepath.Join(absolute, file.name)
		info, err := os.Lstat(path)
		if err != nil {
			return Manifest{}, fmt.Errorf("inspect %s: %w", file.name, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return Manifest{}, fmt.Errorf("%s must be a regular file", file.name)
		}
		actualDigest, actualSize, err := hashFile(path)
		if err != nil {
			return Manifest{}, fmt.Errorf("verify %s: %w", file.name, err)
		}
		if actualDigest != file.digest || actualSize != file.size {
			return Manifest{}, fmt.Errorf("backup integrity check failed for %s", file.name)
		}
	}
	return manifest, nil
}

func RestoreArtifacts(ctx context.Context, directory, destination string) (restoredManifest Manifest, retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	manifest, err := Verify(directory)
	if err != nil {
		return Manifest{}, err
	}
	backupRoot, err := filepath.Abs(directory)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve backup directory: %w", err)
	}
	restoreRoot, err := filepath.Abs(destination)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve artifact restore directory: %w", err)
	}
	if pathsOverlap(backupRoot, restoreRoot) {
		return Manifest{}, errors.New("artifact restore directory must be separate from the backup directory")
	}
	if _, err := os.Lstat(restoreRoot); err == nil {
		return Manifest{}, fmt.Errorf("artifact restore directory already exists: %s", restoreRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Manifest{}, fmt.Errorf("inspect artifact restore directory: %w", err)
	}

	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return Manifest{}, fmt.Errorf("generate artifact restore staging name: %w", err)
	}
	staging := filepath.Join(filepath.Dir(restoreRoot), ".euromni-restore-"+hex.EncodeToString(token))
	if err := privatefs.EnsureDirectory(staging); err != nil {
		return Manifest{}, fmt.Errorf("create private artifact restore staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			if err := os.RemoveAll(staging); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("remove incomplete artifact restore: %w", err))
			}
		}
	}()

	archivePath := filepath.Join(backupRoot, manifest.ArtifactsFile)
	archive, err := os.Open(archivePath)
	if err != nil {
		return Manifest{}, fmt.Errorf("open artifact archive: %w", err)
	}
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return Manifest{}, errors.Join(fmt.Errorf("open artifact archive: %w", err), archive.Close())
	}
	defer func() {
		retErr = errors.Join(retErr, gzipReader.Close(), archive.Close())
	}()
	tarReader := tar.NewReader(gzipReader)
	store, err := artifact.NewFileStore(staging)
	if err != nil {
		return Manifest{}, fmt.Errorf("open artifact restore store: %w", err)
	}
	seen := make(map[string]struct{})
	count := 0
	var totalBytes int64
	for {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, fmt.Errorf("read artifact archive: %w", err)
		}
		parts := strings.Split(header.Name, "/")
		if len(parts) != 2 || !digestPattern.MatchString(parts[1]) ||
			parts[0] != parts[1][:2] || header.Name != parts[0]+"/"+parts[1] ||
			header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > artifact.MaxArtifactBytes {
			return Manifest{}, fmt.Errorf("unsafe artifact archive entry %q", header.Name)
		}
		digest := parts[1]
		if count >= manifest.ArtifactCount {
			return Manifest{}, errors.New("artifact archive exceeds the manifest entry count")
		}
		if _, exists := seen[digest]; exists {
			return Manifest{}, fmt.Errorf("duplicate artifact archive entry %q", header.Name)
		}
		if header.Size > manifest.ArtifactBytes-totalBytes {
			return Manifest{}, errors.New("artifact archive exceeds the manifest byte count")
		}
		stored, err := store.Put(ctx, tarReader)
		if err != nil {
			return Manifest{}, fmt.Errorf("restore artifact %s: %w", digest, err)
		}
		if stored.SHA256 != digest || stored.Size != header.Size {
			return Manifest{}, fmt.Errorf("artifact archive entry %q failed integrity validation", header.Name)
		}
		seen[digest] = struct{}{}
		count++
		totalBytes += stored.Size
	}
	if count != manifest.ArtifactCount || totalBytes != manifest.ArtifactBytes {
		return Manifest{}, errors.New("artifact archive contents do not match the backup manifest")
	}
	if err := os.Rename(staging, restoreRoot); err != nil {
		return Manifest{}, fmt.Errorf("publish restored artifact store: %w", err)
	}
	published = true
	return manifest, nil
}

func Prune(root string, now time.Time) ([]string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve backup directory: %w", err)
	}
	if err := privatefs.EnsureDirectory(absolute); err != nil {
		return nil, fmt.Errorf("ensure private backup directory: %w", err)
	}
	now = now.UTC()
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	backups := make([]backupEntry, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), backupPrefix) {
			continue
		}
		if !entry.IsDir() {
			return nil, fmt.Errorf("refusing to prune non-directory backup entry %q", entry.Name())
		}
		path := filepath.Join(absolute, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect backup entry %q: %w", entry.Name(), err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to prune unsafe backup entry %q", entry.Name())
		}
		manifest, err := readManifest(path)
		if err != nil {
			return nil, fmt.Errorf("refusing retention cleanup because backup %q is invalid: %w", entry.Name(), err)
		}
		matches := backupPattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("refusing retention cleanup for unexpected backup name %q", entry.Name())
		}
		nameTimestamp, err := time.Parse("20060102T150405Z", matches[1])
		if err != nil || !nameTimestamp.Equal(manifest.CreatedAt.UTC()) {
			return nil, fmt.Errorf("backup directory timestamp does not match its manifest: %s", entry.Name())
		}
		backups = append(backups, backupEntry{name: entry.Name(), path: path, createdAt: manifest.CreatedAt.UTC()})
	}
	keep := retainedBackups(backups, now)
	pruned := make([]string, 0)
	for _, entry := range backups {
		if _, retained := keep[entry.name]; retained {
			continue
		}
		if err := os.RemoveAll(entry.path); err != nil {
			return pruned, fmt.Errorf("remove expired backup %q: %w", entry.name, err)
		}
		pruned = append(pruned, entry.name)
	}
	return pruned, nil
}

type backupEntry struct {
	name      string
	path      string
	createdAt time.Time
}

func retainedBackups(backups []backupEntry, now time.Time) map[string]struct{} {
	keep := make(map[string]struct{})
	dailyCutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -13)
	weekStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekday := int(weekStart.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weeklyCutoff := weekStart.AddDate(0, 0, -(weekday-1)).AddDate(0, 0, -7*7)
	monthlyCutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
	daily := map[string]backupEntry{}
	weekly := map[string]backupEntry{}
	monthly := map[string]backupEntry{}
	for _, entry := range backups {
		at := entry.createdAt
		if at.After(now) {
			keep[entry.name] = struct{}{}
			continue
		}
		if !at.Before(dailyCutoff) {
			key := at.Format("2006-01-02")
			daily[key] = newer(daily[key], entry)
		}
		if !at.Before(weeklyCutoff) {
			year, week := at.ISOWeek()
			key := fmt.Sprintf("%04d-%02d", year, week)
			weekly[key] = newer(weekly[key], entry)
		}
		if !at.Before(monthlyCutoff) {
			key := at.Format("2006-01")
			monthly[key] = newer(monthly[key], entry)
		}
	}
	for _, groups := range []map[string]backupEntry{daily, weekly, monthly} {
		for _, entry := range groups {
			keep[entry.name] = struct{}{}
		}
	}
	return keep
}

func newer(current, candidate backupEntry) backupEntry {
	if current.name == "" || candidate.createdAt.After(current.createdAt) {
		return candidate
	}
	return current
}

func readManifest(directory string) (Manifest, error) {
	path := filepath.Join(directory, manifestName)
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, errors.New("backup manifest must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open backup manifest: %w", err)
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	if len(contents) > 64<<10 {
		return Manifest{}, errors.New("backup manifest exceeds size limit")
	}
	var manifest Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode backup manifest: %w", err)
	}
	if manifest.FormatVersion != formatVersion ||
		manifest.CreatedAt.IsZero() ||
		manifest.DatabaseFile != "database.dump" ||
		manifest.ArtifactsFile != "artifacts.tar.gz" ||
		!digestPattern.MatchString(manifest.DatabaseSHA256) ||
		!digestPattern.MatchString(manifest.ArtifactsSHA256) ||
		manifest.DatabaseBytes <= 0 ||
		manifest.ArtifactsBytes <= 0 ||
		manifest.ArtifactCount < 0 ||
		manifest.ArtifactBytes < 0 {
		return Manifest{}, errors.New("backup manifest fields are invalid")
	}
	return manifest, nil
}

func writeManifest(path string, manifest Manifest) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create backup manifest: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return errors.Join(fmt.Errorf("write backup manifest: %w", err), file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync backup manifest: %w", err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close backup manifest: %w", err)
	}
	return nil
}

func archiveArtifacts(ctx context.Context, store *artifact.FileStore, root, destination string) (int, int64, error) {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, 0, fmt.Errorf("create artifact archive: %w", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	count := 0
	var totalBytes int64
	closeArchive := func() error {
		return errors.Join(tarWriter.Close(), gzipWriter.Close(), file.Sync(), file.Close())
	}
	fail := func(err error) (int, int64, error) {
		return 0, 0, errors.Join(err, closeArchive())
	}

	shards, err := os.ReadDir(root)
	if err != nil {
		return fail(fmt.Errorf("list artifact store: %w", err))
	}
	for _, shard := range shards {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if len(shard.Name()) != 2 || !isLowerHex(shard.Name()) {
			continue
		}
		shardPath := filepath.Join(root, shard.Name())
		shardInfo, err := os.Lstat(shardPath)
		if err != nil {
			return fail(fmt.Errorf("inspect artifact shard: %w", err))
		}
		if !shardInfo.IsDir() || shardInfo.Mode()&os.ModeSymlink != 0 {
			return fail(fmt.Errorf("artifact shard %q is not a regular directory", shard.Name()))
		}
		entries, err := os.ReadDir(shardPath)
		if err != nil {
			return fail(fmt.Errorf("list artifact shard: %w", err))
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			digest := entry.Name()
			if !digestPattern.MatchString(digest) || digest[:2] != shard.Name() {
				return fail(fmt.Errorf("unexpected entry in artifact shard %q: %q", shard.Name(), digest))
			}
			path := filepath.Join(shardPath, digest)
			info, err := os.Lstat(path)
			if err != nil {
				return fail(fmt.Errorf("inspect artifact %s: %w", digest, err))
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fail(fmt.Errorf("artifact %s is not a regular file", digest))
			}
			bytes, err := store.Get(ctx, digest)
			if err != nil {
				return fail(fmt.Errorf("verify artifact %s before backup: %w", digest, err))
			}
			header := &tar.Header{
				Name:     filepath.ToSlash(filepath.Join(shard.Name(), digest)),
				Mode:     0o600,
				Size:     int64(len(bytes)),
				ModTime:  info.ModTime().UTC(),
				Typeflag: tar.TypeReg,
			}
			if err := tarWriter.WriteHeader(header); err != nil {
				return fail(fmt.Errorf("write artifact archive header: %w", err))
			}
			if _, err := tarWriter.Write(bytes); err != nil {
				return fail(fmt.Errorf("write artifact archive data: %w", err))
			}
			count++
			totalBytes += int64(len(bytes))
		}
	}
	if err := closeArchive(); err != nil {
		return 0, 0, fmt.Errorf("finish artifact archive: %w", err)
	}
	return count, totalBytes, nil
}

func runPGDump(ctx context.Context, databaseURL, destination string) error {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if config.Database == "" {
		return errors.New("DATABASE_URL must specify a database name")
	}
	environment, err := pgDumpEnvironment(config, databaseURL)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "pg_dump",
		"--no-password",
		"--format=custom",
		"--no-owner",
		"--no-acl",
		"--file", destination,
		"--host", config.Host,
		"--port", fmt.Sprintf("%d", config.Port),
		"--username", config.User,
		"--dbname", config.Database,
	)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("run pg_dump: %w", err)
		}
		return fmt.Errorf("run pg_dump: %w: %s", err, detail)
	}
	return nil
}

func pgDumpEnvironment(config *pgx.ConnConfig, databaseURL string) ([]string, error) {
	keys := map[string]string{
		"sslmode":                  "PGSSLMODE",
		"sslcompression":           "PGSSLCOMPRESSION",
		"sslcert":                  "PGSSLCERT",
		"sslkey":                   "PGSSLKEY",
		"sslpassword":              "PGSSLPASSWORD",
		"sslrootcert":              "PGSSLROOTCERT",
		"sslcrl":                   "PGSSLCRL",
		"sslcrldir":                "PGSSLCRLDIR",
		"ssl_min_protocol_version": "PGSSL_MIN_PROTOCOL_VERSION",
		"ssl_max_protocol_version": "PGSSL_MAX_PROTOCOL_VERSION",
		"gssencmode":               "PGGSSENCMODE",
		"krbsrvname":               "PGKRBSRVNAME",
		"gsslib":                   "PGGSSLIB",
		"connect_timeout":          "PGCONNECT_TIMEOUT",
		"application_name":         "PGAPPNAME",
		"target_session_attrs":     "PGTARGETSESSIONATTRS",
		"channel_binding":          "PGCHANNELBINDING",
		"options":                  "PGOPTIONS",
	}
	values := map[string]string{
		"PGHOST":     config.Host,
		"PGPORT":     fmt.Sprintf("%d", config.Port),
		"PGDATABASE": config.Database,
		"PGUSER":     config.User,
		"PGPASSWORD": config.Password,
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL parameters for pg_dump: %w", err)
	}
	for key, parameters := range parsed.Query() {
		envName, supported := keys[key]
		if !supported {
			switch key {
			case "host", "port", "dbname", "user", "password", "fallback_application_name",
				"replication", "service", "servicefile", "load_balance_hosts":
				return nil, fmt.Errorf("DATABASE_URL parameter %q is not supported by the pg_dump wrapper", key)
			default:
				return nil, fmt.Errorf("DATABASE_URL parameter %q cannot be passed safely to pg_dump", key)
			}
		}
		if len(parameters) != 1 {
			return nil, fmt.Errorf("DATABASE_URL parameter %q must occur once", key)
		}
		values[envName] = parameters[0]
	}
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		upper := strings.ToUpper(name)
		if upper == "DATABASE_URL" || strings.HasPrefix(upper, "PG") {
			continue
		}
		environment = append(environment, variable)
	}
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	sort.Strings(environment)
	return environment, nil
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if samePath(left, right) {
		return true
	}
	return pathContains(left, right) || pathContains(right, left)
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func samePath(left, right string) bool {
	if os.PathSeparator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}
