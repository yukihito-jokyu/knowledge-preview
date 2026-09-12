package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type fakeReadinessChecker struct {
	err error
}

func (f fakeReadinessChecker) Ping(context.Context) error { return f.err }

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()

	router := NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	for _, path := range []string{"/health", "/ready"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			require.Equal(t, http.StatusNoContent, response.Code)
			assert.NotEmpty(t, response.Header().Get(requestIDHeader))
		})
	}
}
