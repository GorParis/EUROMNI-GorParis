package quarantine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreKeepsArchivesPrivateScopedAndVerifiable(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "quarantine"))
	if err != nil {
		t.Fatalf("create quarantine store: %v", err)
	}
	content := []byte("bounded archive fixture")
	first, err := store.Put(context.Background(), "shop-one", content)
	if err != nil {
		t.Fatalf("store archive: %v", err)
	}
	if first.Size != int64(len(content)) || first.SHA256 == "" {
		t.Fatalf("archive receipt = %+v", first)
	}
	repeated, err := store.Put(context.Background(), "shop-one", content)
	if err != nil || repeated != first {
		t.Fatalf("repeat archive receipt = %+v, error = %v; want %+v", repeated, err, first)
	}
	if _, err := store.Get(context.Background(), "shop-two", first.SHA256); err == nil {
		t.Fatal("archive was accessible from another shop scope")
	}
	stored, err := store.Get(context.Background(), "shop-one", first.SHA256)
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("read quarantined archive = %q, error = %v", stored, err)
	}

	archivePath := filepath.Join(store.root, scopeDigest("shop-one"), first.SHA256)
	if err := os.WriteFile(archivePath, []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper with quarantined archive: %v", err)
	}
	if _, err := store.Get(context.Background(), "shop-one", first.SHA256); !errors.Is(err, ErrArchiveCorrupt) {
		t.Fatalf("tampered archive error = %v, want ErrArchiveCorrupt", err)
	}
}

func TestFileStoreRejectsInvalidAndCancelledUploads(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "quarantine"))
	if err != nil {
		t.Fatalf("create quarantine store: %v", err)
	}
	if _, err := store.Put(context.Background(), "shop", nil); !errors.Is(err, ErrArchiveEmpty) {
		t.Fatalf("empty archive error = %v, want ErrArchiveEmpty", err)
	}
	if _, err := store.Put(context.Background(), "shop", make([]byte, MaxArchiveBytes+1)); !errors.Is(err, ErrArchiveLarge) {
		t.Fatalf("large archive error = %v, want ErrArchiveLarge", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(ctx, "shop", []byte("archive")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled upload error = %v, want context.Canceled", err)
	}
}

func TestFileStorePurgesExpiredArchivesAndTemporaryFiles(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "quarantine"))
	if err != nil {
		t.Fatalf("create quarantine store: %v", err)
	}
	oldArchive, err := store.Put(context.Background(), "shop-one", []byte("old archive"))
	if err != nil {
		t.Fatalf("store old archive: %v", err)
	}
	newArchive, err := store.Put(context.Background(), "shop-two", []byte("new archive"))
	if err != nil {
		t.Fatalf("store new archive: %v", err)
	}
	oldPath := filepath.Join(store.root, scopeDigest("shop-one"), oldArchive.SHA256)
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("age old archive: %v", err)
	}
	shopDirectory := filepath.Join(store.root, scopeDigest("shop-two"))
	staleTemporaryPath := filepath.Join(shopDirectory, ".quarantine-stale")
	if err := os.WriteFile(staleTemporaryPath, []byte("stale staging file"), 0o600); err != nil {
		t.Fatalf("write stale staging file: %v", err)
	}
	if err := os.Chtimes(staleTemporaryPath, oldTime, oldTime); err != nil {
		t.Fatalf("age stale staging file: %v", err)
	}
	recentTemporaryPath := filepath.Join(shopDirectory, ".quarantine-recent")
	if err := os.WriteFile(recentTemporaryPath, []byte("active staging file"), 0o600); err != nil {
		t.Fatalf("write recent staging file: %v", err)
	}

	removed, err := store.PurgeBefore(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil || removed != 2 {
		t.Fatalf("purge removed=%d, error=%v; want two removals", removed, err)
	}
	if _, err := store.Get(context.Background(), "shop-one", oldArchive.SHA256); err == nil {
		t.Fatal("expired archive still exists")
	}
	if _, err := os.Stat(staleTemporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staging file still exists, stat error=%v", err)
	}
	if _, err := os.Stat(recentTemporaryPath); err != nil {
		t.Fatalf("recent staging file was removed: %v", err)
	}
	if content, err := store.Get(context.Background(), "shop-two", newArchive.SHA256); err != nil || string(content) != "new archive" {
		t.Fatalf("recent archive content=%q, error=%v", content, err)
	}
}

func TestFileStorePurgeHonorsCancelledContext(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "quarantine"))
	if err != nil {
		t.Fatalf("create quarantine store: %v", err)
	}
	if _, err := store.Put(context.Background(), "shop", []byte("archive")); err != nil {
		t.Fatalf("store archive: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if removed, err := store.PurgeBefore(ctx, time.Now().Add(time.Hour)); !errors.Is(err, context.Canceled) || removed != 0 {
		t.Fatalf("cancelled purge removed=%d, error=%v; want no removals and context.Canceled", removed, err)
	}
}
