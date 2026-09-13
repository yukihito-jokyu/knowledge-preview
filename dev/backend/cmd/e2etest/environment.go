package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/config"
)

func requireTestEnvironment() error {
	if os.Getenv("APP_ENV") != "test" {
		return fmt.Errorf("APP_ENV must be test")
	}

	if strings.TrimSpace(os.Getenv("E2E_RUN_ID")) == "" {
		return fmt.Errorf("E2E_RUN_ID is required")
	}

	return nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.knowledge') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}

	if exists {
		return nil
	}

	path, err := migrationPath()
	if err != nil {
		return err
	}

	migration, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return nil
}

func migrationPath() (string, error) {
	if configured := os.Getenv("E2E_MIGRATIONS_DIR"); configured != "" {
		path := filepath.Join(configured, "000001_knowledge.up.sql")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	for _, path := range []string{
		"/migrations/000001_knowledge.up.sql",
		"migrations/000001_knowledge.up.sql",
		"dev/backend/migrations/000001_knowledge.up.sql",
	} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("knowledge migration file is not available")
}

func ensureBucket(ctx context.Context, cfg config.Knowledge) error {
	client, err := minio.New(
		cfg.Endpoint,
		&minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.Secure},
	)
	if err != nil {
		return err
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{})
}
