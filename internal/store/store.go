package store

import (
	"database/sql"
	"log"
	"os"
	"strings"
	"time"

	"euromni/internal/dbdialect"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store holds the active database connection and SQL dialect.
type Store struct {
	DSN     string
	SQL     *sql.DB
	Dialect dbdialect.Dialect
}

func New(dsn string) *Store {
	return &Store{DSN: dsn, Dialect: dbdialect.PostgreSQL}
}

func NewSQL(db *sql.DB) *Store {
	return &Store{SQL: db, Dialect: dbdialect.PostgreSQL}
}

func NewSQLWithDialect(db *sql.DB, dialect dbdialect.Dialect) *Store {
	return &Store{SQL: db, Dialect: dialect}
}

func Open(dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return &Store{}, nil
	}

	driverName, driverDSN, dialect, err := dbdialect.Resolve(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, driverDSN)
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
	return &Store{DSN: dsn, SQL: db, Dialect: dialect}, nil
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
