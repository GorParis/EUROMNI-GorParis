package store

import (
	"database/sql"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store is the PostgreSQL access layer. Hosting is unspecified; the schema is portable.
type Store struct {
	DSN string
	SQL *sql.DB
}

func New(dsn string) *Store {
	return &Store{DSN: dsn}
}

func NewSQL(db *sql.DB) *Store {
	return &Store{SQL: db}
}

func Open(dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return &Store{}, nil
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &Store{DSN: dsn, SQL: db}, nil
}

func NewFromEnv() *Store {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return &Store{}
	}

	store, err := Open(dsn)
	if err != nil {
		log.Printf("database unavailable, falling back to in-memory store: %v", err)
		return &Store{}
	}
	return store
}

func ProductReaderFromEnv() ProductReader {
	store := NewFromEnv()
	if store == nil || store.SQL == nil {
		return NewMemory()
	}
	return store
}
