package main

import (
	"context"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
)

const actorCookieName = "__Host-kp_e2e_actor"

type actor struct {
	OwnerID   string
	SessionID string
	ID        string
	Display   string
}

type actors struct {
	byCookie map[string]actor
}

func newActors() *actors {
	return &actors{byCookie: map[string]actor{
		"a": {
			OwnerID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			SessionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab",
			ID:        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Display:   "E2E Actor A",
		},
		"b": {
			OwnerID:   "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			SessionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbc",
			ID:        "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			Display:   "E2E Actor B",
		},
	}}
}

func (a *actors) authenticate(c *gin.Context) (domain.Principal, error) {
	identity, ok := a.fromRequest(c)
	if !ok {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	return domain.Principal{OwnerID: identity.OwnerID, SessionID: identity.SessionID}, nil
}

func (a *actors) Active(_ context.Context, p domain.Principal) (bool, error) {
	for _, identity := range a.byCookie {
		if identity.OwnerID == p.OwnerID && identity.SessionID == p.SessionID {
			return true, nil
		}
	}

	return false, nil
}

func (a *actors) fromRequest(c *gin.Context) (actor, bool) {
	value, err := c.Cookie(actorCookieName)
	if err != nil {
		return actor{}, false
	}

	identity, ok := a.byCookie[value]

	return identity, ok
}

func (a *actors) logValues() map[string]map[string]string {
	values := make(map[string]map[string]string, len(a.byCookie))
	for cookie, identity := range a.byCookie {
		values[cookie] = map[string]string{"owner_id": identity.OwnerID, "session_id": identity.SessionID}
	}

	return values
}

func registerAuth(router *gin.Engine, actors *actors, appOrigin string) {
	router.GET("/api/v1/auth/me", func(c *gin.Context) {
		if !sameHost(c, appOrigin) {
			response.WriteError(c, domain.ErrNotFound)
			return
		}

		identity, ok := actors.fromRequest(c)
		if !ok {
			c.JSON(200, nil)
			return
		}

		c.JSON(200, map[string]string{"id": identity.ID, "displayName": identity.Display})
	})
}

func sameHost(c *gin.Context, origin string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && parsed.Host == c.Request.Host
}
