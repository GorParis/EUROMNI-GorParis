package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	"euromni/internal/dbdialect"
	"euromni/internal/migrations"
	schemafiles "euromni/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("db-migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	driverName, driverDSN, dialect, err := dbdialect.Resolve(dsn)
	if err != nil {
		return fmt.Errorf("resolve DATABASE_URL: %w", err)
	}
	var db *sql.DB
	var source fs.FS = schemafiles.Files
	if dialect == dbdialect.MariaDB {
		db, err = sql.Open(driverName, driverDSN)
		if err != nil {
			return fmt.Errorf("open MariaDB DATABASE_URL: %w", err)
		}
		source, err = fs.Sub(schemafiles.MariaDBFiles, "mariadb")
		if err != nil {
			return fmt.Errorf("load embedded MariaDB migrations: %w", err)
		}
	} else {
		config, parseErr := pgx.ParseConfig(dsn)
		if parseErr != nil {
			return fmt.Errorf("parse DATABASE_URL: %w", parseErr)
		}
		config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		db = stdlib.OpenDB(*config)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("close migration database: %v", err)
		}
	}()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("connect to DATABASE_URL: %w", err)
	}
	applied, err := migrations.ApplyWithDialect(ctx, db, source, dialect)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		_, err = fmt.Fprintln(stdout, "Database schema is up to date.")
		return err
	}
	for _, name := range applied {
		if _, err := fmt.Fprintf(stdout, "Applied %s\n", name); err != nil {
			return err
		}
	}
	return nil
}
