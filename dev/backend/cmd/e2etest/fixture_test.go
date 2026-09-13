package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/postgres"
)

func TestInsertDraftIntegration(t *testing.T) {
	database := os.Getenv("KNOWLEDGE_TEST_DATABASE_URL")
	if database == "" {
		t.Skip("set KNOWLEDGE_TEST_DATABASE_URL to a disposable local PostgreSQL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, database)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	schema := "knowledge_e2e_fixture_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, cleanupErr)
	})

	cfg, err := pgxpool.ParseConfig(database)
	require.NoError(t, err)

	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	testPool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(testPool.Close)

	migration, err := os.ReadFile("../../migrations/000001_knowledge.up.sql")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(migration))
	require.NoError(t, err)

	draftID := "33333333-3333-4333-8333-333333333333"
	knowledge := domain.Knowledge{
		OwnerID:        "owner-a",
		Title:          "fixture",
		Format:         "markdown",
		Tags:           []string{"e2e"},
		LearningStatus: "learned",
	}
	require.NoError(t, insertDraft(ctx, testPool, draftID, knowledge, "fixture-source", nil))

	draft, err := postgres.NewKnowledgeRepository(testPool).Draft(ctx, knowledge.OwnerID, draftID)
	require.NoError(t, err)
	require.Equal(t, draftID, draft.DraftID)
	require.Equal(t, knowledge.OwnerID, draft.OwnerID)
	require.Equal(t, knowledge.Title, draft.Title)
	require.Equal(t, knowledge.Format, draft.Format)
	require.Equal(t, knowledge.Tags, draft.Tags)
	require.Equal(t, knowledge.LearningStatus, draft.LearningStatus)
	require.Equal(t, "fixture-source", draft.SourceKey)
	require.EqualValues(t, 1, draft.Version)
	require.Empty(t, draft.CommittedID)
}
