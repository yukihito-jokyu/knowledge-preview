package http

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}

		c.Header(requestIDHeader, requestID)

		startedAt := time.Now()

		c.Next()

		logger.Info("HTTP request completed",
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", time.Since(startedAt).String(),
		)
	}
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "unavailable"
	}

	return hex.EncodeToString(bytes)
}
