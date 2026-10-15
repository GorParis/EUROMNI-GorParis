package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"euromni/internal/api"
	"euromni/internal/store"
)

func main() {
	reader, closeReader, err := configuredProductReader()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := closeReader(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	addr := listenAddress()
	mux := newHandler(reader)
	log.Printf("euromni cms listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func configuredProductReader() (store.ProductReader, func() error, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return store.NewMemory(), func() error { return nil }, nil
	}
	database, err := store.Open(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to DATABASE_URL: %w", err)
	}
	return database, database.SQL.Close, nil
}

func listenAddress() string {
	if address := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); address != "" {
		return address
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}
	return "127.0.0.1:" + port
}

func newHandler(reader store.ProductReader) http.Handler {
	api.SetProductReader(reader)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}
