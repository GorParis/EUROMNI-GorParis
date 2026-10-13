package artifact

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreDeduplicatesAndVerifiesArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-artifacts")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	wasm := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	first, err := store.Put(context.Background(), bytes.NewReader(wasm))
	if err != nil {
		t.Fatalf("put artifact: %v", err)
	}
	second, err := store.Put(context.Background(), bytes.NewReader(wasm))
	if err != nil {
		t.Fatalf("put duplicate artifact: %v", err)
	}
	if first != second {
		t.Fatalf("duplicate metadata = %+v, want %+v", second, first)
	}
	got, err := store.Get(context.Background(), first.SHA256)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if !bytes.Equal(got, wasm) {
		t.Fatalf("got %v, want %v", got, wasm)
	}

	path := filepath.Join(root, first.SHA256[:2], first.SHA256)
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), first.SHA256); !errors.Is(err, ErrArtifactCorrupt) {
		t.Fatalf("tampered artifact error = %v, want %v", err, ErrArtifactCorrupt)
	}
}

func TestFileStoreRejectsInvalidInputsAndPrivateRoot(t *testing.T) {
	if _, err := NewFileStore(""); err == nil {
		t.Fatal("empty root was accepted")
	}
	publicRoot := filepath.Join(t.TempDir(), "public")
	if err := os.Mkdir(publicRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(publicRoot); err == nil {
		t.Fatal("non-private root was accepted")
	}

	store, err := NewFileStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), bytes.NewReader(nil)); !errors.Is(err, ErrArtifactEmpty) {
		t.Fatalf("empty artifact error = %v", err)
	}
	if _, err := store.Get(context.Background(), "../secret"); !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("invalid digest error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(canceled, bytes.NewReader([]byte("artifact"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled put error = %v", err)
	}
}

func TestFileStoreEnforcesUploadSizeLimit(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.put(context.Background(), bytes.NewReader([]byte("12345678")), 7)
	if !errors.Is(err, ErrArtifactTooBig) {
		t.Fatalf("oversized artifact error = %v, want %v", err, ErrArtifactTooBig)
	}
}

func TestFileStorePrunesOnlyOldUnreferencedArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-artifacts")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Put(context.Background(), bytes.NewReader([]byte("active wasm")))
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.Put(context.Background(), bytes.NewReader([]byte("orphan wasm")))
	if err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(root, orphan.SHA256[:2], orphan.SHA256)
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphanPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	removed, err := store.PruneUnreferenced(context.Background(), []string{active.SHA256}, 24*time.Hour)
	if err != nil {
		t.Fatalf("PruneUnreferenced() returned error: %v", err)
	}
	if len(removed) != 1 || removed[0] != orphan {
		t.Fatalf("removed = %#v, want only orphan %+v", removed, orphan)
	}
	if _, err := store.Get(context.Background(), active.SHA256); err != nil {
		t.Fatalf("referenced artifact was removed: %v", err)
	}
	if _, err := store.Get(context.Background(), orphan.SHA256); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan read error = %v, want os.ErrNotExist", err)
	}
}

func TestFileStorePrunePreservesRecentAndRecentlyReusedArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-artifacts")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	recentBytes := []byte("recent wasm")
	recent, err := store.Put(context.Background(), bytes.NewReader(recentBytes))
	if err != nil {
		t.Fatal(err)
	}
	recentPath := filepath.Join(root, recent.SHA256[:2], recent.SHA256)
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(recentPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), bytes.NewReader(recentBytes)); err != nil {
		t.Fatalf("reuse artifact: %v", err)
	}
	removed, err := store.PruneUnreferenced(context.Background(), nil, 24*time.Hour)
	if err != nil {
		t.Fatalf("PruneUnreferenced() returned error: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %#v, want recently reused artifact preserved", removed)
	}
}

func TestFileStorePruneRejectsInvalidReferenceWithoutDeleting(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "private-artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), bytes.NewReader([]byte("wasm"))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PruneUnreferenced(context.Background(), []string{"../escape"}, time.Hour); !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("prune error = %v, want ErrInvalidDigest", err)
	}
}
