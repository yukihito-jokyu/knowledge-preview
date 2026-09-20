package http

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type authHTTPRepository struct {
	states map[string]struct {
		verifier string
		state    usecase.OAuthState
	}
	audits []string
}

func newAuthHTTPRepository() *authHTTPRepository {
	return &authHTTPRepository{states: make(map[string]struct {
		verifier string
		state    usecase.OAuthState
	})}
}

func (r *authHTTPRepository) SaveOAuthState(_ context.Context, hash, verifier string, state usecase.OAuthState) error {
	r.states[hash] = struct {
		verifier string
		state    usecase.OAuthState
	}{verifier: verifier, state: state}

	return nil
}

func (r *authHTTPRepository) ConsumeOAuthState(_ context.Context, hash, verifier string) (usecase.OAuthState, error) {
	value, ok := r.states[hash]
	if !ok || value.verifier != verifier {
		return usecase.OAuthState{}, domain.ErrBadRequest
	}

	delete(r.states, hash)

	return value.state, nil
}

func (r *authHTTPRepository) UpsertUser(context.Context, usecase.OAuthUser) error { return nil }

func (r *authHTTPRepository) CreateRefreshSession(context.Context, usecase.RefreshSession, string, time.Time) error {
	return nil
}

func (r *authHTTPRepository) RotateRefreshToken(
	context.Context,
	string,
	string,
	time.Time,
) (usecase.RefreshSession, error) {
	return usecase.RefreshSession{}, domain.ErrUnauthenticated
}

func (r *authHTTPRepository) RevokeSession(context.Context, string) error { return nil }

func (r *authHTTPRepository) RevokeRefreshToken(context.Context, string) (string, error) {
	return "42", nil
}

func (r *authHTTPRepository) Active(context.Context, domain.Principal) (bool, error) {
	return true, nil
}

func (r *authHTTPRepository) Audit(_ context.Context, event, _ string) error {
	r.audits = append(r.audits, event)
	return nil
}

type authHTTPProvider struct {
	user      usecase.OAuthUser
	state     string
	verifier  string
	challenge string
}

func (p *authHTTPProvider) AuthorizationURL(state, challenge string) string {
	p.state = state
	p.challenge = challenge

	return "https://oauth.test/authorize?state=" + url.QueryEscape(state)
}

func (p *authHTTPProvider) Exchange(_ context.Context, _, verifier string) (usecase.OAuthUser, error) {
	p.verifier = verifier
	return p.user, nil
}

func newAuthHTTPRouter(
	repo *authHTTPRepository,
	provider *authHTTPProvider,
) *gin.Engine {
	router := NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	auth := usecase.NewAuthUseCase(repo, provider, usecase.AuthSettings{
		AllowedGitHubIDs: map[string]struct{}{"42": {}},
		SigningKey:       []byte(strings.Repeat("k", 32)),
		Issuer:           "issuer",
		Audience:         "audience",
	})
	RegisterAuth(router, auth, "https://app.test")

	return router
}

func authRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, "https://app.test"+target, body)
	request.Host = "app.test"

	return request
}

func TestAuthHTTPStartBindsCallbackBrowserAndReturnsJSON(t *testing.T) {
	repo := newAuthHTTPRepository()
	provider := &authHTTPProvider{user: usecase.OAuthUser{ID: "42", DisplayName: "User"}}
	router := newAuthHTTPRouter(repo, provider)

	start := authRequest(
		http.MethodPost,
		"/api/v1/auth/github/start",
		strings.NewReader(`{"returnTo":"/knowledge/new"}`),
	)
	start.Header.Set("Origin", "https://app.test")

	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, start)

	require.Equal(t, http.StatusOK, startResponse.Code)
	require.Contains(t, startResponse.Body.String(), `"authorizeUrl"`)
	cookie := startResponse.Result().Cookies()[0]
	require.Equal(t, oauthCookie, cookie.Name)
	assert.True(t, cookie.Secure)
	assert.True(t, cookie.HttpOnly)

	state := provider.state
	require.NotEmpty(t, state)
	require.NotEqual(t, state, cookie.Value)
	digest := sha256.Sum256([]byte(cookie.Value))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(digest[:]), provider.challenge)

	otherBrowser := authRequest(
		http.MethodGet,
		"/api/v1/auth/github/callback?code=code&state="+url.QueryEscape(state),
		nil,
	)
	otherResponse := httptest.NewRecorder()
	router.ServeHTTP(otherResponse, otherBrowser)
	require.Equal(t, http.StatusSeeOther, otherResponse.Code)
	require.Equal(t, "/login?error=oauth_invalid", otherResponse.Header().Get("Location"))
	assert.Empty(t, responseCookie(otherResponse, accessCookie))

	callback := authRequest(http.MethodGet, "/api/v1/auth/github/callback?code=code&state="+url.QueryEscape(state), nil)
	callback.AddCookie(cookie)

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, callback)
	require.Equal(t, http.StatusSeeOther, callbackResponse.Code)
	require.Equal(t, "/knowledge/new", callbackResponse.Header().Get("Location"))
	assert.NotEmpty(t, responseCookie(callbackResponse, accessCookie))
	assert.Equal(t, cookie.Value, provider.verifier)
}

func TestAuthHTTPCallbackFailureRedirectsWithoutProviderDetails(t *testing.T) {
	repo := newAuthHTTPRepository()
	provider := &authHTTPProvider{user: usecase.OAuthUser{ID: "42"}}
	router := newAuthHTTPRouter(repo, provider)

	start := authRequest(http.MethodPost, "/api/v1/auth/github/start", strings.NewReader(`{}`))
	start.Header.Set("Origin", "https://app.test")

	startResponse := httptest.NewRecorder()
	router.ServeHTTP(startResponse, start)
	cookie := startResponse.Result().Cookies()[0]

	callback := authRequest(
		http.MethodGet,
		"/api/v1/auth/github/callback?state="+url.QueryEscape(
			provider.state,
		)+"&error=access_denied&error_description=secret",
		nil,
	)
	callback.AddCookie(cookie)

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, callback)

	require.Equal(t, http.StatusSeeOther, callbackResponse.Code)
	require.Equal(t, "/login?error=oauth_denied", callbackResponse.Header().Get("Location"))
	assert.NotContains(t, callbackResponse.Header().Get("Location"), "secret")
	assert.Contains(t, repo.audits, "callback_denied")
}

func TestAuthHTTPProcessRateLimitSharesStartAndCallback(t *testing.T) {
	router := newAuthHTTPRouter(newAuthHTTPRepository(), &authHTTPProvider{user: usecase.OAuthUser{ID: "42"}})

	limited := false

	for i := 0; i < authProcessRateLimitBurst+100; i++ {
		start := authRequest(http.MethodPost, "/api/v1/auth/github/start", strings.NewReader(`{}`))
		start.Header.Set("Origin", "https://app.test")
		start.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i%250+1))

		startResponse := httptest.NewRecorder()
		router.ServeHTTP(startResponse, start)

		if startResponse.Code == http.StatusTooManyRequests {
			limited = true

			assert.Equal(t, "60", startResponse.Header().Get("Retry-After"))

			break
		}
	}

	require.True(t, limited)

	callback := authRequest(http.MethodGet, "/api/v1/auth/github/callback", nil)
	callback.Header.Set("X-Forwarded-For", "203.0.113.1")

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, callback)

	require.Equal(t, http.StatusTooManyRequests, callbackResponse.Code)
	assert.Equal(t, "rate_limited", responseErrorCode(callbackResponse))
}

func TestAuthProcessRateLimiterStopsAtCapacity(t *testing.T) {
	limiter := newAuthProcessRateLimiter(2, time.Hour)

	assert.True(t, limiter.allow())
	assert.True(t, limiter.allow())
	assert.False(t, limiter.allow())
}

func responseCookie(response *httptest.ResponseRecorder, name string) string {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie.Value
		}
	}

	return ""
}

func responseErrorCode(response *httptest.ResponseRecorder) string {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		return ""
	}

	return body.Error.Code
}
