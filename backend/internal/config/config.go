package config

import "os"

type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

func Load() Config {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddr:    os.Getenv("HTTP_ADDR"),
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "postgres://explorer:explorer@localhost:5432/explainab?sslmode=disable"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	return cfg
}
