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

	"euromni/internal/quarantine"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("quarantine-reconciler", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("dir", strings.TrimSpace(os.Getenv("EUROMNI_QUARANTINE_DIR")), "private quarantine directory")
	olderThan := flags.Duration("older-than", 0, "remove quarantine files older than this Go duration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("-dir or EUROMNI_QUARANTINE_DIR is required")
	}
	if *olderThan <= 0 {
		return errors.New("-older-than must be a positive Go duration")
	}
	info, err := os.Lstat(*root)
	if err != nil {
		return fmt.Errorf("inspect quarantine directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("quarantine path must be an existing directory")
	}

	store, err := quarantine.NewFileStore(*root)
	if err != nil {
		return fmt.Errorf("open quarantine store: %w", err)
	}
	removed, err := store.PurgeBefore(context.Background(), time.Now().Add(-*olderThan))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "quarantine purge complete: removed=%d older_than=%s\n", removed, *olderThan)
	return err
}