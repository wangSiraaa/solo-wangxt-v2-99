package main

import (
	"context"
	"log"
	"time"

	"github.com/example/explainab/internal/config"
	"github.com/example/explainab/internal/db"
	"github.com/example/explainab/internal/httpapi"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer repo.DB.Close()
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := db.SeedDefaults(ctx, repo); err != nil {
		log.Fatalf("seed: %v", err)
	}

	r := httpapi.NewServer(repo)
	log.Printf("ExplainAB listening on %s", cfg.HTTPAddr)
	if err := r.Run(cfg.HTTPAddr); err != nil {
		log.Fatal(err)
	}
}
