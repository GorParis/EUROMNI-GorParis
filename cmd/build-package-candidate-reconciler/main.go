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

	"euromni/internal/packagecandidate"
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

	candidates, err := packagecandidate.NewFileStore(*root)
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
