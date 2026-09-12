package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

func NewRouter(readiness usecase.ReadinessUseCase, logger *slog.Logger) *gin.Engine {
	router := gin.New()
	// 信頼するプロキシを設定するまで無効化する
	_ = router.SetTrustedProxies(nil)
	router.Use(requestLogger(logger), gin.Recovery())

	health := newHealthHandler(readiness)
	router.GET("/health", health.live)
	router.GET("/ready", health.ready)

	return router
}
