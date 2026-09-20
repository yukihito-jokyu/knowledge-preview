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

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/config"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/github"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/htmlsafe"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/postgres"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/s3"
	httpinterface "github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/http"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration is invalid", "error", err)
		os.Exit(1)
	}

	if err := cfg.Auth.Validate(cfg.Environment); err != nil {
		logger.Error("authentication configuration is invalid", "error", err)
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

	readiness := usecase.NewReadinessUseCase(pool)
	router := httpinterface.NewRouter(readiness, logger)
	auth := usecase.NewAuthUseCase(
		postgres.NewAuthRepository(pool),
		github.New(
			cfg.Auth.ClientID,
			cfg.Auth.ClientSecret,
			cfg.Auth.RedirectURL,
			cfg.Auth.AuthorizeURL,
			cfg.Auth.TokenURL,
			cfg.Auth.UserURL,
		),
		usecase.AuthSettings{
			AllowedGitHubIDs: cfg.Auth.AllowedGitHubIDs,
			SigningKey:       cfg.Auth.SigningKey,
			Issuer:           cfg.Auth.Issuer,
			Audience:         cfg.Auth.Audience,
			AccessTTL:        cfg.Auth.AccessTTL,
			RefreshTTL:       cfg.Auth.RefreshTTL,
			StateTTL:         cfg.Auth.StateTTL,
		},
	)
	httpinterface.RegisterAuth(router, auth, cfg.Auth.AppOrigin)

	authenticate := func(c *gin.Context) (domain.Principal, error) {
		value, err := c.Cookie("__Host-kp_access")
		if err != nil {
			return domain.Principal{}, domain.ErrUnauthenticated
		}

		return auth.Authenticate(c.Request.Context(), value)
	}

	knowledgeConfig, err := config.LoadKnowledge()
	if err != nil {
		logger.Error("knowledge configuration is invalid", "error", err)
		os.Exit(1)
	}

	var objects usecase.KnowledgeObjects = s3.UnavailableObjects{}
	if knowledgeConfig.Endpoint != "" {
		objects, err = s3.New(
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
	}

	knowledge := usecase.NewKnowledgeUseCase(
		postgres.NewKnowledgeRepository(pool),
		objects,
		htmlsafe.New(),
		auth,
	)
	httpinterface.RegisterKnowledge(
		router,
		knowledge,
		authenticate,
		knowledgeConfig.AppOrigin,
		knowledgeConfig.PreviewOrigin,
	)
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
		logger.Info("HTTP server started", "address", cfg.HTTPAddr, "environment", cfg.Environment)

		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}
