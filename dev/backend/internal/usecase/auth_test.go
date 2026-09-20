package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

type authRepositoryFake struct {
	states  map[string]OAuthState
	users   map[string]OAuthUser
	refresh map[string]RefreshSession
	revoked map[string]bool
	audits  []string
}

func newAuthRepositoryFake() *authRepositoryFake {
	return &authRepositoryFake{
		states:  map[string]OAuthState{},
		users:   map[string]OAuthUser{},
		refresh: map[string]RefreshSession{},
		revoked: map[string]bool{},
	}
}

func (r *authRepositoryFake) SaveOAuthState(_ context.Context, hash, _ string, state OAuthState) error {
	r.states[hash] = state
	return nil
}

func (r *authRepositoryFake) ConsumeOAuthState(_ context.Context, hash, _ string) (OAuthState, error) {
	state, ok := r.states[hash]
	if !ok || !state.Expires.After(time.Now()) {
		return OAuthState{}, domain.ErrBadRequest
	}

	delete(r.states, hash)

	return state, nil
}

func (r *authRepositoryFake) UpsertUser(_ context.Context, user OAuthUser) error {
	r.users[user.ID] = user
	return nil
}

func (r *authRepositoryFake) CreateRefreshSession(
	_ context.Context,
	session RefreshSession,
	hash string,
	_ time.Time,
) error {
	r.refresh[hash] = session
	return nil
}

func (r *authRepositoryFake) RotateRefreshToken(
	_ context.Context,
	hash, nextHash string,
	expires time.Time,
) (RefreshSession, error) {
	session, ok := r.refresh[hash]
	if !ok {
		return RefreshSession{}, domain.ErrUnauthenticated
	}

	if r.revoked[hash] {
		r.revoked[session.FamilyID] = true
		return session, domain.ErrRefreshReuse
	}

	if r.revoked[session.FamilyID] {
		return session, domain.ErrUnauthenticated
	}

	if !session.Expires.After(time.Now()) {
		return session, domain.ErrUnauthenticated
	}

	r.revoked[hash] = true
	r.refresh[nextHash] = RefreshSession{
		SessionID: session.SessionID,
		FamilyID:  session.FamilyID,
		UserID:    session.UserID,
		Expires:   expires,
	}

	return session, nil
}

func (r *authRepositoryFake) RevokeSession(_ context.Context, sessionID string) error {
	for hash, session := range r.refresh {
		if session.SessionID == sessionID {
			r.revoked[hash] = true
		}
	}

	return nil
}

func (r *authRepositoryFake) RevokeRefreshToken(_ context.Context, hash string) (string, error) {
	session, ok := r.refresh[hash]
	if !ok {
		return "", domain.ErrNotFound
	}

	r.revoked[hash] = true
	r.revoked[session.FamilyID] = true

	return session.UserID, nil
}

func (r *authRepositoryFake) Active(_ context.Context, principal domain.Principal) (bool, error) {
	for hash, session := range r.refresh {
		if session.UserID == principal.OwnerID && session.SessionID == principal.SessionID && !r.revoked[hash] &&
			!r.revoked[session.FamilyID] {
			return true, nil
		}
	}

	return false, nil
}

func (r *authRepositoryFake) Audit(_ context.Context, event, _ string) error {
	r.audits = append(r.audits, event)
	return nil
}

type authOAuthFake struct {
	user OAuthUser
}

func (o authOAuthFake) AuthorizationURL(state, challenge string) string {
	return "https://oauth.test/authorize?state=" + url.QueryEscape(
		state,
	) + "&code_challenge=" + url.QueryEscape(
		challenge,
	)
}

func (o authOAuthFake) Exchange(context.Context, string, string) (OAuthUser, error) {
	return o.user, nil
}

func authTestUseCase(repo *authRepositoryFake, provider OAuthProvider) *AuthUseCase {
	return NewAuthUseCase(repo, provider, AuthSettings{
		AllowedGitHubIDs: map[string]struct{}{"42": {}},
		SigningKey:       []byte(strings.Repeat("k", 32)),
		Issuer:           "issuer",
		Audience:         "audience",
	})
}

func TestAuthPKCEAndOneTimeState(t *testing.T) {
	repo := newAuthRepositoryFake()
	provider := authOAuthFake{user: OAuthUser{ID: "42", DisplayName: "User"}}
	u := authTestUseCase(repo, provider)

	location, verifier, err := u.Start(context.Background(), "/knowledge/new")
	require.NoError(t, err)
	parsed, err := url.Parse(location)
	require.NoError(t, err)

	state := parsed.Query().Get("state")
	challenge := parsed.Query().Get("code_challenge")

	require.Len(t, state, 43)
	require.Len(t, verifier, 43)
	require.NotEqual(t, state, verifier)
	digest := sha256.Sum256([]byte(verifier))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(digest[:]), challenge)

	access, refresh, returnTo, _, err := u.Callback(context.Background(), "code", state, verifier)
	require.NoError(t, err)
	require.Equal(t, "/knowledge/new", returnTo)
	require.NotEmpty(t, access)
	require.Len(t, refresh, 43)

	_, _, _, _, err = u.Callback(context.Background(), "code", state, verifier)
	require.ErrorIs(t, err, domain.ErrBadRequest)
}

func TestAuthRefreshRotationAndReuse(t *testing.T) {
	repo := newAuthRepositoryFake()
	u := authTestUseCase(repo, authOAuthFake{user: OAuthUser{ID: "42"}})
	location, verifier, err := u.Start(context.Background(), "/")
	require.NoError(t, err)
	state := mustQuery(t, location, "state")
	_, refresh, _, _, err := u.Callback(context.Background(), "code", state, verifier)
	require.NoError(t, err)

	_, next, err := u.Refresh(context.Background(), refresh)
	require.NoError(t, err)
	_, _, err = u.Refresh(context.Background(), refresh)
	require.ErrorIs(t, err, domain.ErrRefreshReuse)
	_, _, err = u.Refresh(context.Background(), next)
	require.ErrorIs(t, err, domain.ErrUnauthenticated)
}

func TestAuthLogoutWithRefreshOnlyAuditsOwner(t *testing.T) {
	repo := newAuthRepositoryFake()
	u := authTestUseCase(repo, authOAuthFake{user: OAuthUser{ID: "42"}})
	location, verifier, err := u.Start(context.Background(), "/")
	require.NoError(t, err)
	state := mustQuery(t, location, "state")
	_, refresh, _, _, err := u.Callback(context.Background(), "code", state, verifier)
	require.NoError(t, err)

	require.NoError(t, u.Logout(context.Background(), "", refresh))
	require.Contains(t, repo.audits, "logout")
}

func TestAuthRejectsExpiredAndFutureJWT(t *testing.T) {
	u := authTestUseCase(newAuthRepositoryFake(), authOAuthFake{user: OAuthUser{ID: "42"}})
	principal := domain.Principal{OwnerID: "42", SessionID: "session"}

	expired, err := u.signAccess(principal, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	_, err = u.VerifyAccess(expired, time.Now())
	require.ErrorIs(t, err, domain.ErrUnauthenticated)

	future, err := u.signAccess(principal, time.Now().Add(2*time.Minute))
	require.NoError(t, err)
	_, err = u.VerifyAccess(future, time.Now())
	require.ErrorIs(t, err, domain.ErrUnauthenticated)
}

func mustQuery(t *testing.T, raw, key string) string {
	t.Helper()

	parsed, err := url.Parse(raw)
	require.NoError(t, err)

	return parsed.Query().Get(key)
}
