package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"euromni/internal/store"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("staff-bootstrap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	shopID := flags.String("shop", "", "shop UUID")
	email := flags.String("email", "", "owner email address")
	name := flags.String("name", "", "owner display name")
	passwordStdin := flags.Bool("password-stdin", false, "read the password from standard input")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if strings.TrimSpace(*shopID) == "" || strings.TrimSpace(*email) == "" || strings.TrimSpace(*name) == "" || !*passwordStdin {
		return errors.New("required flags: --shop, --email, --name, --password-stdin")
	}

	passwordBytes, err := io.ReadAll(io.LimitReader(stdin, 74))
	if err != nil {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	if len(passwordBytes) > 72 {
		return errors.New("password must be at most 72 bytes")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(passwordBytes), "\n"), "\r")
	if len(password) < 12 {
		return errors.New("password must be at least 12 bytes")
	}

	db, err := store.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if db.SQL == nil {
		return errors.New("DATABASE_URL is required")
	}
	bootstrapErr := db.BootstrapStaff(context.Background(), strings.TrimSpace(*shopID), strings.TrimSpace(*email), strings.TrimSpace(*name), password)
	closeErr := db.SQL.Close()
	if bootstrapErr != nil {
		return errors.Join(fmt.Errorf("bootstrap owner account: %w", bootstrapErr), closeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close database connection: %w", closeErr)
	}
	fmt.Fprintf(stdout, "Created the initial owner account for shop %s.\n", strings.TrimSpace(*shopID))
	return nil
}
