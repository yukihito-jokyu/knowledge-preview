package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/s3"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
)

const fixtureBodyLimit = 6*domain.MaxSourceBytes + 4096

type fixtureRequest struct {
	Owner    string `json:"owner"`
	Format   string `json:"format"`
	Source   string `json:"source"`
	AgeYears *int   `json:"ageYears"`
}

type fixtureResponse struct {
	DraftID string `json:"draftId"`
}

func registerFixtures(router *gin.Engine, pool *pgxpool.Pool, objects *s3.Objects, actors *actors, appOrigin string) {
	router.POST("/api/v1/__e2e/fixtures", func(c *gin.Context) {
		if !sameHost(c, appOrigin) {
			response.WriteError(c, domain.ErrNotFound)
			return
		}

		var request fixtureRequest
		if !decodeFixture(c, &request) {
			return
		}

		owner, ok := fixtureOwner(actors, request.Owner)
		if !ok || (request.Format != "markdown" && request.Format != "html") ||
			request.AgeYears != nil && (*request.AgeYears < 0 || *request.AgeYears > 100) {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		knowledge := domain.Knowledge{
			OwnerID:        owner,
			Title:          "E2E fixture",
			Format:         request.Format,
			Tags:           []string{},
			LearningStatus: "unlearned",
		}

		knowledge, _, err := domain.ApplySource(knowledge, request.Source)
		if err != nil {
			response.WriteError(c, err)
			return
		}

		draftID, sourceKey, err := newFixtureIDs()
		if err != nil {
			response.WriteError(c, domain.ErrUnavailable)
			return
		}

		if err := objects.Put(c.Request.Context(), sourceKey, request.Source); err != nil {
			response.WriteError(c, err)
			return
		}

		if err := insertDraft(c.Request.Context(), pool, draftID, knowledge, sourceKey, request.AgeYears); err != nil {
			_ = objects.Delete(c.Request.Context(), sourceKey)
			response.WriteError(c, err)

			return
		}

		c.JSON(http.StatusCreated, fixtureResponse{DraftID: draftID})
	})
}

func decodeFixture(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, fixtureBodyLimit)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			response.WriteError(c, domain.ErrTooLarge)
		} else {
			response.WriteError(c, domain.ErrBadRequest)
		}

		return false
	}

	if !utf8.Valid(data) {
		response.WriteError(c, domain.ErrBadRequest)
		return false
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		response.WriteError(c, domain.ErrBadRequest)
		return false
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.WriteError(c, domain.ErrBadRequest)
		return false
	}

	return true
}

func fixtureOwner(actors *actors, value string) (string, bool) {
	identity, ok := actors.byCookie[value]
	if !ok {
		return "", false
	}

	return identity.OwnerID, true
}

func insertDraft(
	ctx context.Context,
	pool *pgxpool.Pool,
	draftID string,
	knowledge domain.Knowledge,
	sourceKey string,
	ageYears *int,
) error {
	createdAt := time.Now()
	if ageYears != nil {
		createdAt = createdAt.AddDate(-*ageYears, 0, 0)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key,state,created_at) VALUES($1,'ready',$2)`,
		sourceKey,
		createdAt,
	)
	if err != nil {
		return domain.ErrUnavailable
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO knowledge_drafts(id,owner_id,title,format,tags,learning_status,source_key) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		draftID,
		knowledge.OwnerID,
		knowledge.Title,
		knowledge.Format,
		knowledge.Tags,
		knowledge.LearningStatus,
		sourceKey,
	)
	if err != nil {
		return domain.ErrUnavailable
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}

	return nil
}

func newFixtureIDs() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}

	draftID := uuidFromBytes(bytes[:16])
	runID := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}

		return '-'
	}, strings.TrimSpace("e2e-"+strings.TrimSpace(os.Getenv("E2E_RUN_ID"))))

	return draftID, runID + "/knowledge/" + hex.EncodeToString(bytes[16:]), nil
}

func uuidFromBytes(bytes []byte) string {
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return hex.EncodeToString(
		bytes[0:4],
	) + "-" + hex.EncodeToString(
		bytes[4:6],
	) + "-" + hex.EncodeToString(
		bytes[6:8],
	) + "-" + hex.EncodeToString(
		bytes[8:10],
	) + "-" + hex.EncodeToString(
		bytes[10:16],
	)
}
