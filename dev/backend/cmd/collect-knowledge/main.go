package main

import (
	"context"
	"log"
	"time"

	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/config"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/postgres"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/s3"
)

func main() {
	if err := run(); err != nil {
		log.Fatal("knowledge collection failed; retry after checking dependencies")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	storage, err := config.LoadKnowledge()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	objects, err := s3.New(storage.Endpoint, storage.Bucket, storage.AccessKey, storage.SecretKey, storage.Secure)
	if err != nil {
		return err
	}

	repository := postgres.NewKnowledgeRepository(pool)

	count, err := repository.Collect(ctx, time.Now().Add(-24*time.Hour), objects.Delete)
	if err != nil {
		return err
	}

	log.Printf("collected %d unreferenced objects", count)

	return nil
}
