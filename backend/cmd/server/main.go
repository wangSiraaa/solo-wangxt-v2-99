package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/example/ab-platform/internal/api"
	"github.com/example/ab-platform/internal/storage"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var store storage.Store
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		pg, err := storage.NewPostgresStore(ctx, databaseURL)
		if err != nil {
			log.Fatalf("connect PostgreSQL: %v", err)
		}
		defer pg.Close()
		store = pg
		log.Printf("using PostgreSQL")
	} else {
		store = storage.NewMemoryStore(storage.SeedConfig())
		log.Printf("DATABASE_URL not set; using non-persistent in-memory seed (for local UI development only)")
	}

	addr := ":" + env("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.NewServer(store).Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(srv.ListenAndServe())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
