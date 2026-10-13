package main

import (
	"log"
	"net/http"
	"os"

	"euromni/internal/api"
	"euromni/internal/store"
)

func main() {
	api.SetProductReader(store.ProductReaderFromEnv())

	mux := http.NewServeMux()
	api.Register(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := ":" + port
	log.Printf("euromni cms listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
