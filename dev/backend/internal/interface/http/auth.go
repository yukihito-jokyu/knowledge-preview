package http

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

const (
	accessCookie  = "__Host-kp_access"
	refreshCookie = "__Host-kp_refresh"
	oauthCookie   = "__Host-kp_oauth"
)

type authHandler struct {
	usecase   *usecase.AuthUseCase
	appOrigin string
}

func RegisterAuth(router *gin.Engine, u *usecase.AuthUseCase, appOrigin string) {
	h := &authHandler{usecase: u, appOrigin: appOrigin}
	group := router.Group("/api/v1/auth", h.host)
	group.POST("/github/start", h.origin, h.start)
	group.GET("/github/callback", h.callback)
	group.GET("/session", h.session)
	group.POST("/refresh", h.origin, h.refresh)
	group.POST("/logout", h.origin, h.logout)
}

func (h *authHandler) host(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	if h.appOrigin != "" {
		origin, err := url.Parse(h.appOrigin)
		if err != nil || origin.Host != c.Request.Host {
			response.WriteError(c, domain.ErrNotFound)
			return
		}
	}

	c.Next()
}

func (h *authHandler) origin(c *gin.Context) {
	if h.appOrigin == "" || c.GetHeader("Origin") != h.appOrigin {
		response.WriteError(c, domain.ErrForbidden)
		return
	}

	c.Next()
}

func (h *authHandler) start(c *gin.Context) {
	returnTo := "/"

	if c.Request.ContentLength > 0 {
		var body struct {
			ReturnTo string `json:"returnTo"`
		}
		if !decode(c, &body) {
			return
		}

		returnTo = body.ReturnTo
	}

	location, verifier, err := h.usecase.Start(c.Request.Context(), returnTo)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	setOAuthCookie(c, verifier, h.usecase.StateLifetime())
	c.JSON(http.StatusOK, response.Authorization{URL: location})
}

func (h *authHandler) callback(c *gin.Context) {
	state := c.Query("state")

	verifier, _ := c.Cookie(oauthCookie)
	if c.Query("error") != "" {
		h.usecase.RejectCallback(c.Request.Context(), state, verifier, "callback_denied")
		clearOAuthCookie(c)
		redirectLogin(c, "oauth_denied")

		return
	}

	access, refresh, returnTo, _, err := h.usecase.Callback(
		c.Request.Context(),
		c.Query("code"),
		state,
		verifier,
	)
	clearOAuthCookie(c)

	if err != nil {
		redirectLogin(c, callbackErrorCode(err))
		return
	}

	setAuthCookies(c, access, refresh, h.usecase.AccessLifetime(), h.usecase.RefreshLifetime())
	c.Redirect(http.StatusSeeOther, returnTo)
}

func (h *authHandler) session(c *gin.Context) {
	value, err := c.Cookie(accessCookie)
	if err != nil {
		c.JSON(http.StatusOK, nil)
		return
	}

	viewer, err := h.usecase.Viewer(c.Request.Context(), value)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Viewer{ID: viewer.ID, DisplayName: viewer.DisplayName, Email: viewer.Email})
}

func (h *authHandler) refresh(c *gin.Context) {
	value, err := c.Cookie(refreshCookie)
	if err != nil {
		response.WriteError(c, domain.ErrUnauthenticated)
		return
	}

	access, refresh, err := h.usecase.Refresh(c.Request.Context(), value)
	if err != nil {
		clearAuthCookies(c)
		response.WriteError(c, err)

		return
	}

	setAuthCookies(c, access, refresh, h.usecase.AccessLifetime(), h.usecase.RefreshLifetime())
	c.Status(http.StatusNoContent)
}

func (h *authHandler) logout(c *gin.Context) {
	access, _ := c.Cookie(accessCookie)

	refresh, _ := c.Cookie(refreshCookie)
	if err := h.usecase.Logout(c.Request.Context(), access, refresh); err != nil {
		response.WriteError(c, err)
		return
	}

	clearAuthCookies(c)
	c.Status(http.StatusNoContent)
}

func setAuthCookies(c *gin.Context, access, refresh string, accessLifetime, refreshLifetime time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(accessCookie, access, int(accessLifetime.Seconds()), "/", "", true, true)
	c.SetCookie(refreshCookie, refresh, int(refreshLifetime.Seconds()), "/", "", true, true)
}

func setOAuthCookie(c *gin.Context, verifier string, lifetime time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthCookie, verifier, int(lifetime.Seconds()), "/", "", true, true)
}

func clearAuthCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)

	for _, name := range []string{accessCookie, refreshCookie, oauthCookie} {
		c.SetCookie(name, "", -1, "/", "", true, true)
	}
}

func clearOAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthCookie, "", -1, "/", "", true, true)
}

func redirectLogin(c *gin.Context, code string) {
	c.Redirect(http.StatusSeeOther, "/login?error="+url.QueryEscape(code))
}

func callbackErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, domain.ErrForbidden):
		return "oauth_forbidden"
	case errors.Is(err, domain.ErrOAuth):
		return "oauth_unavailable"
	case errors.Is(err, domain.ErrUnavailable):
		return "oauth_unavailable"
	default:
		return "oauth_invalid"
	}
}
