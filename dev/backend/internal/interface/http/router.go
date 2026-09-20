package http

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

const (
	// authProcessRateLimitBurst absorbs traffic from many users when the boundary proxy is absent.
	authProcessRateLimitBurst  = 600
	authProcessRateLimitWindow = time.Minute
)

type authProcessRateLimiter struct {
	mu              sync.Mutex
	tokens          float64
	capacity        float64
	refillPerSecond float64
	lastRefill      time.Time
}

func newAuthProcessRateLimiter(capacity int, window time.Duration) *authProcessRateLimiter {
	return &authProcessRateLimiter{
		tokens:          float64(capacity),
		capacity:        float64(capacity),
		refillPerSecond: float64(capacity) / window.Seconds(),
		lastRefill:      time.Now(),
	}
}

func (l *authProcessRateLimiter) allow() bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.tokens += now.Sub(l.lastRefill).Seconds() * l.refillPerSecond
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}

	l.lastRefill = now

	if l.tokens < 1 {
		return false
	}

	l.tokens--

	return true
}

func authProcessRateLimit(limiter *authProcessRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isAuthRateLimitedPath(c.Request.URL.Path) || limiter.allow() {
			c.Next()
			return
		}

		c.Header("Retry-After", "60")
		response.WriteError(c, domain.ErrRateLimited)
	}
}

func isAuthRateLimitedPath(path string) bool {
	return path == "/api/v1/auth/github/start" || path == "/api/v1/auth/github/callback"
}

func NewRouter(readiness usecase.ReadinessUseCase, logger *slog.Logger) *gin.Engine {
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}

	router.Use(
		requestLogger(logger),
		gin.Recovery(),
		authProcessRateLimit(newAuthProcessRateLimiter(authProcessRateLimitBurst, authProcessRateLimitWindow)),
	)

	health := newHealthHandler(readiness)
	router.GET("/health", health.live)
	router.GET("/ready", health.ready)

	return router
}
