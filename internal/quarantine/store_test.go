package quarantine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
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
