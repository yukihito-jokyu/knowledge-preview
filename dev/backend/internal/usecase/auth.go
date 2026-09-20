package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

// AuthSettings contains authentication settings that are safe to pass into the use case.
type AuthSettings struct {
	AllowedGitHubIDs map[string]struct{}
	SigningKey       []byte
	Issuer           string
	Audience         string
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	StateTTL         time.Duration
}

type (
	OAuthUser      = domain.OAuthUser
	OAuthState     = domain.OAuthState
	RefreshSession = domain.RefreshSession
)

type AuthRepository interface {
	SaveOAuthState(context.Context, string, string, OAuthState) error
	ConsumeOAuthState(context.Context, string, string) (OAuthState, error)
	UpsertUser(context.Context, OAuthUser) error
	CreateRefreshSession(context.Context, RefreshSession, string, time.Time) error
	RotateRefreshToken(context.Context, string, string, time.Time) (RefreshSession, error)
	RevokeSession(context.Context, string) error
	RevokeRefreshToken(context.Context, string) (string, error)
	Active(context.Context, domain.Principal) (bool, error)
	Audit(context.Context, string, string) error
}

type OAuthProvider interface {
	AuthorizationURL(state, challenge string) string
	Exchange(context.Context, string, string) (OAuthUser, error)
}

type AuthUseCase struct {
	repo     AuthRepository
	oauth    OAuthProvider
	settings AuthSettings
}

func NewAuthUseCase(repo AuthRepository, oauth OAuthProvider, settings AuthSettings) *AuthUseCase {
	if settings.AccessTTL <= 0 {
		settings.AccessTTL = 15 * time.Minute
	}

	if settings.RefreshTTL <= 0 {
		settings.RefreshTTL = 30 * 24 * time.Hour
	}

	if settings.StateTTL <= 0 {
		settings.StateTTL = 10 * time.Minute
	}

	return &AuthUseCase{repo: repo, oauth: oauth, settings: settings}
}

func (u *AuthUseCase) AccessLifetime() time.Duration  { return u.settings.AccessTTL }
func (u *AuthUseCase) RefreshLifetime() time.Duration { return u.settings.RefreshTTL }
func (u *AuthUseCase) StateLifetime() time.Duration   { return u.settings.StateTTL }

func (u *AuthUseCase) Start(ctx context.Context, returnTo string) (string, string, error) {
	if u.repo == nil || u.oauth == nil || len(u.settings.SigningKey) < 32 {
		return "", "", domain.ErrUnavailable
	}

	returnTo = safeReturnTo(returnTo)

	state, err := randomAuthToken(32)
	if err != nil {
		return "", "", domain.ErrUnavailable
	}

	verifier, err := randomAuthToken(32)
	if err != nil {
		return "", "", domain.ErrUnavailable
	}
	// State and the PKCE verifier are independent. Only hashes are persisted.
	stateHash := hashToken(state)
	if err := u.repo.SaveOAuthState(
		ctx,
		stateHash,
		hashToken(verifier),
		OAuthState{ReturnTo: returnTo, Expires: time.Now().Add(u.settings.StateTTL)},
	); err != nil {
		return "", "", err
	}

	challenge := base64.RawURLEncoding.EncodeToString(sha256Bytes([]byte(verifier)))

	return u.oauth.AuthorizationURL(state, challenge), verifier, nil
}

func (u *AuthUseCase) Callback(
	ctx context.Context,
	code, state, verifier string,
) (string, string, string, OAuthUser, error) {
	if state == "" || verifier == "" {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_invalid", domain.ErrBadRequest)
	}

	record, err := u.repo.ConsumeOAuthState(ctx, hashToken(state), hashToken(verifier))
	if err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_invalid", err)
	}

	if code == "" {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_invalid", domain.ErrBadRequest)
	}

	user, err := u.oauth.Exchange(ctx, code, verifier)
	if err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_oauth", domain.ErrOAuth)
	}

	if _, ok := u.settings.AllowedGitHubIDs[user.ID]; !ok {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_forbidden", domain.ErrForbidden)
	}

	if err := u.repo.UpsertUser(ctx, user); err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", err)
	}

	sessionID, err := randomUUID()
	if err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", domain.ErrUnavailable)
	}

	familyID, err := randomUUID()
	if err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", domain.ErrUnavailable)
	}

	refresh, err := randomAuthToken(32)
	if err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", domain.ErrUnavailable)
	}

	expires := time.Now().Add(u.settings.RefreshTTL)
	if err := u.repo.CreateRefreshSession(
		ctx,
		RefreshSession{SessionID: sessionID, FamilyID: familyID, UserID: user.ID, Expires: expires},
		hashToken(refresh),
		expires,
	); err != nil {
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", err)
	}

	access, err := u.signAccess(domain.Principal{OwnerID: user.ID, SessionID: sessionID}, time.Now())
	if err != nil {
		_ = u.repo.RevokeSession(ctx, sessionID)
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", domain.ErrUnavailable)
	}

	if err := u.repo.Audit(ctx, "login", user.ID); err != nil {
		_ = u.repo.RevokeSession(ctx, sessionID)
		return "", "", "", OAuthUser{}, u.callbackFailure(ctx, "callback_internal", err)
	}

	return access, refresh, record.ReturnTo, user, nil
}

// RejectCallback consumes a matching one-time state and records a classified failure.
func (u *AuthUseCase) RejectCallback(ctx context.Context, state, verifier, event string) {
	if state != "" && verifier != "" {
		_, _ = u.repo.ConsumeOAuthState(ctx, hashToken(state), hashToken(verifier))
	}

	_ = u.repo.Audit(ctx, event, "")
}

func (u *AuthUseCase) callbackFailure(ctx context.Context, event string, err error) error {
	_ = u.repo.Audit(ctx, event, "")
	return err
}

func (u *AuthUseCase) Refresh(ctx context.Context, refresh string) (string, string, error) {
	if refresh == "" {
		return "", "", domain.ErrUnauthenticated
	}

	next, err := randomAuthToken(32)
	if err != nil {
		return "", "", domain.ErrUnavailable
	}

	session, err := u.repo.RotateRefreshToken(
		ctx,
		hashToken(refresh),
		hashToken(next),
		time.Now().Add(u.settings.RefreshTTL),
	)
	if err != nil {
		if errors.Is(err, domain.ErrRefreshReuse) && session.UserID != "" {
			_ = u.repo.Audit(ctx, "refresh_reuse", session.UserID)
		}

		return "", "", err
	}

	access, err := u.signAccess(domain.Principal{OwnerID: session.UserID, SessionID: session.SessionID}, time.Now())
	if err != nil {
		_ = u.repo.RevokeSession(ctx, session.SessionID)
		return "", "", domain.ErrUnavailable
	}

	if err := u.repo.Audit(ctx, "refresh", session.UserID); err != nil {
		_ = u.repo.RevokeSession(ctx, session.SessionID)
		return "", "", err
	}

	return access, next, nil
}

func (u *AuthUseCase) Logout(ctx context.Context, access, refresh string) error {
	var errs []error

	var ownerID string

	if refresh != "" {
		refreshOwnerID, err := u.repo.RevokeRefreshToken(ctx, hashToken(refresh))
		if err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrUnauthenticated) {
			errs = append(errs, err)
		}

		if err == nil {
			ownerID = refreshOwnerID
		}
	}

	if access != "" {
		principal, verifyErr := u.VerifyAccess(access, time.Now())
		if verifyErr == nil {
			ownerID = principal.OwnerID
			if err := u.repo.RevokeSession(ctx, principal.SessionID); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if ownerID != "" {
		if err := u.repo.Audit(ctx, "logout", ownerID); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (u *AuthUseCase) VerifyAccess(token string, now time.Time) (domain.Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	mac := hmac.New(sha256.New, u.settings.SigningKey)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))

	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	var header struct {
		Algorithm string `json:"alg"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "HS256" {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	var claims accessClaims
	if json.Unmarshal(payloadBytes, &claims) != nil || claims.Issuer != u.settings.Issuer ||
		claims.Audience != u.settings.Audience ||
		claims.Expires <= now.Unix() ||
		claims.Issued > now.Add(time.Minute).Unix() ||
		claims.Subject == "" ||
		claims.Session == "" ||
		claims.ID == "" {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	return domain.Principal{OwnerID: claims.Subject, SessionID: claims.Session}, nil
}

func (u *AuthUseCase) Authenticate(ctx context.Context, token string) (domain.Principal, error) {
	principal, err := u.VerifyAccess(token, time.Now())
	if err != nil {
		return domain.Principal{}, err
	}

	active, err := u.repo.Active(ctx, principal)
	if err != nil {
		return domain.Principal{}, domain.ErrUnavailable
	}

	if !active {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	return principal, nil
}

func (u *AuthUseCase) Viewer(ctx context.Context, token string) (OAuthUser, error) {
	principal, err := u.Authenticate(ctx, token)
	if err != nil {
		return OAuthUser{}, err
	}

	reader, ok := u.repo.(interface {
		User(context.Context, string) (OAuthUser, error)
	})
	if !ok {
		return OAuthUser{ID: principal.OwnerID}, nil
	}

	return reader.User(ctx, principal.OwnerID)
}

func (u *AuthUseCase) Active(ctx context.Context, principal domain.Principal) (bool, error) {
	return u.repo.Active(ctx, principal)
}

func (u *AuthUseCase) signAccess(principal domain.Principal, now time.Time) (string, error) {
	id, err := randomAuthToken(16)
	if err != nil {
		return "", err
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payload, err := json.Marshal(accessClaims{
		Issuer:   u.settings.Issuer,
		Audience: u.settings.Audience,
		Subject:  principal.OwnerID,
		Session:  principal.SessionID,
		Issued:   now.Unix(),
		Expires:  now.Add(u.settings.AccessTTL).Unix(),
		ID:       id,
	})
	if err != nil {
		return "", err
	}

	encoded := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, u.settings.SigningKey)
	_, _ = mac.Write([]byte(encoded))

	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

type accessClaims struct {
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Subject  string `json:"sub"`
	Session  string `json:"sid"`
	Issued   int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	ID       string `json:"jti"`
}

func safeReturnTo(value string) string {
	if value == "" {
		return "/"
	}

	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}

	return u.RequestURI()
}

func randomAuthToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:]), nil
}

func sha256Bytes(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}

func hashToken(value string) string {
	return hex.EncodeToString(sha256Bytes([]byte(value)))
}
