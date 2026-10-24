package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"euromni/internal/dbdialect"
	"euromni/internal/packagecandidate"
	"euromni/internal/store"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("build-package-candidate-reconciler", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String(
		"dir",
		strings.TrimSpace(os.Getenv("EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR")),
		"private signed build package candidate directory",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("-dir or EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR is required")
	}
	info, err := os.Lstat(*root)
	if err != nil {
		return fmt.Errorf("inspect build package candidate directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("build package candidate path must be an existing directory")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required for shared candidate quota coordination")
	}
	database, err := store.Open(databaseURL)
	if err != nil {
		return fmt.Errorf("open PostgreSQL candidate coordinator: %w", err)
	}
	if database.SQL == nil || database.Dialect != dbdialect.PostgreSQL {
		if database.SQL != nil {
			if err := database.SQL.Close(); err != nil {
				return errors.Join(errors.New("candidate reconciliation requires PostgreSQL"), fmt.Errorf("close database: %w", err))
			}
		}
		return errors.New("candidate reconciliation requires PostgreSQL")
	}
	purgeErr := purge(*root, database, stdout)
	if err := database.SQL.Close(); err != nil {
		return errors.Join(purgeErr, fmt.Errorf("close PostgreSQL candidate coordinator: %w", err))
	}
	return purgeErr
}

func purge(root string, database *store.Store, stdout io.Writer) error {
	if database == nil || database.SQL == nil || database.Dialect != dbdialect.PostgreSQL {
		return errors.New("candidate reconciliation requires PostgreSQL")
	}
	candidates, err := packagecandidate.NewCoordinatedFileStore(root, database.SQL)
	if err != nil {
		return fmt.Errorf("open build package candidate store: %w", err)
	}
	removed, err := candidates.PurgeRejectedBefore(
		context.Background(),
		time.Now().Add(-packagecandidate.RejectedCandidateRetention),
	)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"build package candidate purge complete: removed=%d retention=%s\n",
		removed,
		packagecandidate.RejectedCandidateRetention,
	)
	return err
}
