package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type healthHandler struct {
	readiness usecase.ReadinessUseCase
}

func newHealthHandler(readiness usecase.ReadinessUseCase) healthHandler {
	return healthHandler{readiness: readiness}
}

// live はプロセスの生存確認。
func (h healthHandler) live(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// ready は依存先を含む準備確認。
func (h healthHandler) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.readiness.Execute(ctx); err != nil {
		response.WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
