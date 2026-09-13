package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/htmlsafe"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type handlerRepository struct{ usecase.KnowledgeRepository }

func (handlerRepository) Get(_ context.Context, owner, id string) (domain.Knowledge, error) {
	if owner != "owner" {
		return domain.Knowledge{}, domain.ErrNotFound
	}

	return domain.Knowledge{
		ID:             id,
		OwnerID:        owner,
		Title:          "title",
		Format:         "markdown",
		Tags:           []string{},
		LearningStatus: "unlearned",
		Version:        1,
		Visibility:     "private",
		SourceKey:      "source",
	}, nil
}

type handlerObjects struct{}

func (handlerObjects) Get(context.Context, string) (string, error) { return "# body", nil }
func (handlerObjects) Put(context.Context, string, string) error   { return nil }

type handlerSessions struct{}

func (handlerSessions) Active(context.Context, domain.Principal) (bool, error) { return true, nil }
func TestKnowledgeHTTPBoundary(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"

	u := usecase.NewKnowledgeUseCase(handlerRepository{}, handlerObjects{}, htmlsafe.New(), handlerSessions{})

	for _, test := range []struct {
		name, method, path, body, origin, host string
		authenticated                          bool
		want                                   int
	}{
		{
			name:   "no authentication",
			method: "GET",
			path:   "/api/v1/knowledge/" + id,
			want:   401,
		},
		{
			name:          "detail",
			method:        "GET",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			want:          200,
		},
		{
			name:          "bad ID",
			method:        "GET",
			path:          "/api/v1/knowledge/not-an-id",
			authenticated: true,
			want:          400,
		},
		{
			name:          "cross origin",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://evil.test",
			body:          `{"version":1,"source":"new"}`,
			want:          403,
		},
		{
			name:          "unknown field",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":"new","ownerId":"other"}`,
			want:          400,
		},
		{
			name:          "missing source",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1}`,
			want:          400,
		},
		{
			name:          "invalid version",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":0,"source":"new"}`,
			want:          400,
		},
		{
			name:          "empty source",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":" "}`,
			want:          422,
		},
		{
			name:          "trailing YAML",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":"---\ntitle: first\n...\nowner: other\n---\nbody"}`,
			want:          422,
		},
		{
			name:          "stale version",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":2,"source":"new"}`,
			want:          409,
		},
		{
			name:          "multiple documents",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":"new"}{}`,
			want:          400,
		},
		{
			name:   "preview on app host",
			method: "GET",
			path:   "/private/html?ticket=invalid",
			want:   404,
		},
		{
			name:   "invalid preview",
			method: "GET",
			path:   "/private/html?ticket=invalid",
			host:   "preview.test",
			want:   404,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter(
				usecase.NewReadinessUseCase(fakeReadinessChecker{}),
				slog.New(slog.NewTextHandler(io.Discard, nil)),
			)

			var auth Authenticate
			if test.authenticated {
				auth = func(*gin.Context) (domain.Principal, error) {
					return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
				}
			}

			RegisterKnowledge(router, u, auth, "https://app.test", "https://preview.test")

			host := test.host
			if host == "" {
				host = "app.test"
			}

			request := httptest.NewRequest(test.method, "https://"+host+test.path, strings.NewReader(test.body))
			request.Header.Set("Origin", test.origin)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.want, recorder.Code, recorder.Body.String())
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			require.Empty(t, recorder.Header().Get("Set-Cookie"))
			require.Empty(t, recorder.Header().Get("ETag"))

			if test.host == "preview.test" {
				require.Contains(
					t,
					recorder.Header().Get("Content-Security-Policy"),
					"frame-ancestors https://app.test",
				)
				require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
			}

			if test.want == http.StatusOK {
				require.NotContains(t, recorder.Body.String(), "sourceKey")
				require.NotContains(t, recorder.Body.String(), "ownerId")
			}
		})
	}
}
