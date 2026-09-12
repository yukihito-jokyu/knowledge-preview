package config

import (
	"fmt"
	"os"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultEnvironment = "development"
)

// Config は環境変数から読み込む設定。
type Config struct {
	Environment string
	HTTPAddr    string
	DatabaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		Environment: valueOrDefault("APP_ENV", defaultEnvironment),
		HTTPAddr:    valueOrDefault("HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
