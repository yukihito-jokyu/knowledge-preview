package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

const (
	faultDatabase = "database"
	faultObject   = "object"
)

type faultController struct {
	mu        sync.Mutex
	remaining map[string]int
}

func newFaultController() *faultController {
	return &faultController{remaining: make(map[string]int)}
}

func (f *faultController) set(kind string, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remaining[kind] = count
}

func (f *faultController) consume(kind string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.remaining[kind] < 1 {
		return false
	}
	f.remaining[kind]--
	return true
}

type faultRepository struct {
	usecase.KnowledgeRepository
	faults *faultController
}

func (r *faultRepository) CreateDraft(ctx context.Context, draft domain.Draft) error {
	if r.faults.consume(faultDatabase) {
		return domain.ErrUnavailable
	}
	return r.KnowledgeRepository.CreateDraft(ctx, draft)
}

type faultObjects struct {
	usecase.KnowledgeObjects
	faults *faultController
}

func (o *faultObjects) Put(ctx context.Context, key, source string) error {
	if o.faults.consume(faultObject) {
		return domain.ErrUnavailable
	}
	return o.KnowledgeObjects.Put(ctx, key, source)
}

type faultRequest struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type draftStateResponse struct {
	Pending int64 `json:"pending"`
	Total   int64 `json:"total"`
}

func registerTestControls(
	router *gin.Engine,
	pool *pgxpool.Pool,
	actors *actors,
	appOrigin string,
	faults *faultController,
) {
	router.POST("/api/v1/__e2e/faults", func(c *gin.Context) {
		if !sameHost(c, appOrigin) || !authenticatedActor(c, actors) {
			response.WriteError(c, domain.ErrNotFound)
			return
		}

		var request faultRequest
		if !decodeFixture(c, &request) || (request.Kind != faultDatabase && request.Kind != faultObject) || request.Count < 1 || request.Count > 10 {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		faults.set(request.Kind, request.Count)
		c.Status(http.StatusNoContent)
	})

	router.GET("/api/v1/__e2e/draft-state", func(c *gin.Context) {
		if !sameHost(c, appOrigin) || !authenticatedActor(c, actors) {
			response.WriteError(c, domain.ErrNotFound)
			return
		}

		owner, ok := fixtureOwner(actors, c.Query("owner"))
		if !ok {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		var state draftStateResponse
		if err := pool.QueryRow(
			c.Request.Context(),
			`SELECT COUNT(*) FILTER (WHERE committed_id IS NULL), COUNT(*) FROM knowledge_drafts WHERE owner_id=$1`,
			owner,
		).Scan(&state.Pending, &state.Total); err != nil {
			response.WriteError(c, domain.ErrUnavailable)
			return
		}

		c.JSON(http.StatusOK, state)
	})
}

func authenticatedActor(c *gin.Context, actors *actors) bool {
	_, ok := actors.fromRequest(c)
	return ok
}
