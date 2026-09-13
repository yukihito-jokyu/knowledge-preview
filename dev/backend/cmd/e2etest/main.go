// Package main は業務E2E環境専用の実行プログラム。
// テスト用認証が製品バイナリへ誤って含まれないよう、cmd/apiから分離する。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/config"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/htmlsafe"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/postgres"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/s3"
	httpinterface "github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/http"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := requireTestEnvironment(); err != nil {
		logger.Error("test server configuration is invalid", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration is invalid", "error", err)
		os.Exit(1)
	}

	knowledgeConfig, err := config.LoadKnowledge()
	if err != nil {
		logger.Error("knowledge configuration is invalid", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := applyMigration(ctx, pool); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	objects, err := s3.New(
		knowledgeConfig.Endpoint,
		knowledgeConfig.Bucket,
		knowledgeConfig.AccessKey,
		knowledgeConfig.SecretKey,
		knowledgeConfig.Secure,
	)
	if err != nil {
		logger.Error("object storage configuration is invalid")
		os.Exit(1)
	}

	if err := ensureBucket(ctx, knowledgeConfig); err != nil {
		logger.Error("object storage bucket is unavailable", "error", err)
		os.Exit(1)
	}

	actors := newActors()
	readiness := usecase.NewReadinessUseCase(pool)
	router := httpinterface.NewRouter(readiness, logger)
	knowledge := usecase.NewKnowledgeUseCase(postgres.NewKnowledgeRepository(pool), objects, htmlsafe.New(), actors)
	httpinterface.RegisterKnowledge(
		router,
		knowledge,
		actors.authenticate,
		knowledgeConfig.AppOrigin,
		knowledgeConfig.PreviewOrigin,
	)
	registerAuth(router, actors, knowledgeConfig.AppOrigin)
	registerFixtures(router, pool, objects, actors, knowledgeConfig.AppOrigin)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("E2E test server started", "address", cfg.HTTPAddr, "run_id", os.Getenv("E2E_RUN_ID"))
		logger.Info("E2E actor identities configured", "cookie", actorCookieName, "actors", actors.logValues())

		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("test server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("test server shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}
