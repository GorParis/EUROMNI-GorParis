package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"euromni/internal/backup"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("db-backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	verify := flags.String("verify", "", "verify the integrity of an existing backup directory")
	restoreArtifacts := flags.String("restore-artifacts", "", "restore the FileStore from a backup directory")
	artifactDir := flags.String("artifact-dir", "", "new destination directory for restored artifacts")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *verify != "" && *restoreArtifacts != "" {
		return errors.New("-verify and -restore-artifacts cannot be used together")
	}
	if *verify != "" {
		if *artifactDir != "" {
			return errors.New("-artifact-dir is only valid with -restore-artifacts")
		}
		manifest, err := backup.Verify(*verify)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Backup verified: %s\nCreated: %s\n", *verify, manifest.CreatedAt.Format(time.RFC3339))
		return err
	}
	if *restoreArtifacts != "" {
		if strings.TrimSpace(*artifactDir) == "" {
			return errors.New("-artifact-dir is required with -restore-artifacts")
		}
		manifest, err := backup.RestoreArtifacts(ctx, *restoreArtifacts, *artifactDir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Restored %d artifacts (%d bytes) to %s\n",
			manifest.ArtifactCount, manifest.ArtifactBytes, *artifactDir)
		return err
	}
	if *artifactDir != "" {
		return errors.New("-artifact-dir is only valid with -restore-artifacts")
	}

	result, err := backup.Create(ctx, backup.Options{
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		ArtifactDir: strings.TrimSpace(os.Getenv("EUROMNI_ARTIFACT_DIR")),
		BackupDir:   strings.TrimSpace(os.Getenv("EUROMNI_BACKUP_DIR")),
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Backup created: %s\n", result.Directory); err != nil {
		return err
	}
	for _, name := range result.Pruned {
		if _, err := fmt.Fprintf(stdout, "Pruned expired backup: %s\n", name); err != nil {
			return err
		}
	}
	return nil
}
