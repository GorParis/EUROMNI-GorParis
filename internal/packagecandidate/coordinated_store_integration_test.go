//go:build integration

package packagecandidate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestCoordinatedFileStoresEnforceShopQuotaAcrossInstances(t *testing.T) {
	db := testdb.Start(t)
	root := filepath.Join(t.TempDir(), "shared-candidates")
	first, err := NewCoordinatedFileStore(root, db)
	if err != nil {
		t.Fatalf("create first coordinated store: %v", err)
	}
	second, err := NewCoordinatedFileStore(root, db)
	if err != nil {
		t.Fatalf("create second coordinated store: %v", err)
	}
	directory := filepath.Join(root, shopDigest(seed.ShopID))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create shop candidate directory: %v", err)
	}
	occupiedPath := filepath.Join(directory, strings.Repeat("a", 64))
	occupied, err := os.Create(occupiedPath)
	if err != nil {
		t.Fatalf("create existing candidate fixture: %v", err)
	}
	if err := occupied.Truncate(240 << 20); err != nil {
		_ = occupied.Close()
		t.Fatalf("size existing candidate fixture: %v", err)
	}
	if err := occupied.Close(); err != nil {
		t.Fatalf("close existing candidate fixture: %v", err)
	}

	type result struct {
		receipt Candidate
		err     error
	}
	results := make(chan result, 2)
	var start sync.WaitGroup
	start.Add(1)
	var writers sync.WaitGroup
	writers.Add(2)
	for i, store := range []*FileStore{first, second} {
		go func(i int, store *FileStore) {
			defer writers.Done()
			start.Wait()
			content := []byte(strings.Repeat(string(rune('a'+i)), 10<<20))
			receipt, err := store.Put(context.Background(), seed.ShopID, content)
			results <- result{receipt: receipt, err: err}
		}(i, store)
	}
	start.Done()
	writers.Wait()
	close(results)

	stored, rejected := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			stored++
		case errors.Is(result.err, ErrCandidateStoreFull):
			rejected++
		default:
			t.Fatalf("concurrent Put() error = %v, want success or quota rejection", result.err)
		}
	}
	if stored != 1 || rejected != 1 {
		t.Fatalf("concurrent results stored=%d rejected=%d; want one each", stored, rejected)
	}

	used, err := candidateDirectorySize(directory)
	if err != nil {
		t.Fatalf("measure shared candidate directory: %v", err)
	}
	if used > MaxShopCandidateBytes {
		t.Fatalf("shared candidate directory uses %d bytes, exceeds %d-byte quota", used, MaxShopCandidateBytes)
	}
}
