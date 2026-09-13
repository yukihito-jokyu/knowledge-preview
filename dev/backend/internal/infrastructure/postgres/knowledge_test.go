package postgres_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/htmlsafe"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/postgres"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type memoryObjects struct {
	mu     sync.Mutex
	values map[string]string
	fail   bool
}

func (m *memoryObjects) Put(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return domain.ErrUnavailable
	}

	m.values[key] = value

	return nil
}

func (m *memoryObjects) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return "", domain.ErrUnavailable
	}

	value, ok := m.values[key]
	if !ok {
		return "", domain.ErrUnavailable
	}

	return value, nil
}

func (m *memoryObjects) remove(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.values, key)

	return nil
}

type sessions struct {
	active      bool
	unavailable bool
}

func (s *sessions) Active(_ context.Context, p domain.Principal) (bool, error) {
	if s.unavailable {
		return false, domain.ErrUnavailable
	}

	return s.active && p.SessionID == "session-a", nil
}

// このテスト専用の新規スキーマで、実際のPostgreSQLのロックと制約を検証する。
// URLには使い捨てのローカルDBを指定する。アプリの環境設定は参照しない。
func TestKnowledgeIntegration(t *testing.T) {
	database := os.Getenv("KNOWLEDGE_TEST_DATABASE_URL")
	if database == "" {
		t.Skip("set KNOWLEDGE_TEST_DATABASE_URL to a disposable local PostgreSQL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, database)
	require.NoError(t, err)

	defer pool.Close()

	schema := "knowledge_test_" + time.Now().Format("20060102150405")
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema)

	require.NoError(t, err)
	defer func() { _, err := pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); require.NoError(t, err) }()

	cfg, err := pgxpool.ParseConfig(database)
	require.NoError(t, err)

	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	testPool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)

	defer testPool.Close()

	migration, err := os.ReadFile("../../../migrations/000001_knowledge.up.sql")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(migration))
	require.NoError(t, err)

	repo := postgres.NewKnowledgeRepository(testPool)
	objects := &memoryObjects{values: map[string]string{"draft-source": "# old"}}
	session := &sessions{active: true}
	u := usecase.NewKnowledgeUseCase(repo, objects, htmlsafe.New(), session)
	owner := domain.Principal{OwnerID: "owner-a", SessionID: "session-a"}
	draftID := "11111111-1111-4111-8111-111111111111"
	_, err = testPool.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key,created_at) VALUES('draft-source',now()-interval '10 years'); INSERT INTO knowledge_drafts(id,owner_id,title,format,source_key) VALUES('11111111-1111-4111-8111-111111111111','owner-a','draft','markdown','draft-source')`,
	)
	require.NoError(t, err)
	_, _, err = u.Draft(ctx, domain.Principal{OwnerID: "owner-b", SessionID: "session-a"}, draftID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	// 10年前のdraftも利用でき、未参照オブジェクトの回収後も本文が残る。
	_, err = repo.Collect(ctx, time.Now(), objects.remove)
	require.NoError(t, err)
	_, source, err := u.Draft(ctx, owner, draftID)
	require.NoError(t, err)
	require.Equal(t, "# old", source)

	type result struct {
		id      string
		created bool
		err     error
	}

	results := make(chan result, 2)

	for range 2 {
		go func() {
			id, created, err := u.Commit(ctx, owner, draftID, 1, "# committed")
			results <- result{id, created, err}
		}()
	}

	a, b := <-results, <-results
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	require.Equal(t, a.id, b.id)
	require.NotEqual(t, a.created, b.created)
	id := a.id
	_, _, err = u.Commit(ctx, owner, draftID, 1, "different")
	require.ErrorIs(t, err, domain.ErrConflict)
	d, _, err := u.Draft(ctx, owner, draftID)
	require.NoError(t, err)
	require.Equal(t, id, d.CommittedID)
	k, _, err := u.Get(ctx, owner, id)
	require.NoError(t, err)
	require.EqualValues(t, 1, k.Version)
	require.False(t, k.HTMLSanitized)
	k, _, err = u.Visibility(ctx, owner, id, 1, "unlisted")
	require.NoError(t, err)

	publicID := *k.PublicID
	require.EqualValues(t, 2, k.Version)
	k, _, err = u.Visibility(ctx, owner, id, 2, "private")
	require.NoError(t, err)
	_, err = repo.Public(ctx, publicID, 3)
	require.ErrorIs(t, err, domain.ErrNotFound)
	k, _, err = u.Visibility(ctx, owner, id, 3, "unlisted")
	require.NoError(t, err)
	require.Equal(t, publicID, *k.PublicID)

	_, _, err = u.Save(ctx, owner, id, 3, "stale")
	require.ErrorIs(t, err, domain.ErrConflict)

	objects.fail = true
	_, _, err = u.Save(ctx, owner, id, 4, "lost")
	require.ErrorIs(t, err, domain.ErrUnavailable)

	objects.fail = false
	k, source, err = u.Get(ctx, owner, id)
	require.NoError(t, err)
	require.EqualValues(t, 4, k.Version)
	require.Equal(t, "# committed", source)

	k, _, err = u.Save(ctx, owner, id, 4, "---\ntitle: updated\n---\n# new")
	require.NoError(t, err)
	require.EqualValues(t, 5, k.Version)
	require.Equal(t, "updated", k.Title)

	_, err = repo.Public(ctx, publicID, 4)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = repo.Public(ctx, publicID, 5)
	require.NoError(t, err)
	// #15が作成するHTMLのdraftを用意し、以降は公開されたユースケースのメソッドだけを使う。
	htmlID := "22222222-2222-4222-8222-222222222222"
	_, err = testPool.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key) VALUES('html-draft'); INSERT INTO knowledge_drafts(id,owner_id,title,format,source_key) VALUES('22222222-2222-4222-8222-222222222222','owner-a','html','html','html-draft')`,
	)
	require.NoError(t, err)
	htmlKnowledge, _, err := u.Commit(ctx, owner, htmlID, 1, `<p>safe</p><script>alert(1)</script>`)
	require.NoError(t, err)
	replayed, created, err := u.Commit(ctx, owner, htmlID, 1, `<p>safe</p><script>alert(1)</script>`)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, htmlKnowledge, replayed)

	recovered, _, err := u.Draft(ctx, owner, htmlID)
	require.NoError(t, err)
	require.Equal(t, htmlKnowledge, recovered.CommittedID)
	savedHTML, savedSource, err := u.Get(ctx, owner, recovered.CommittedID)
	require.NoError(t, err)
	require.True(t, savedHTML.HTMLSanitized)
	require.Equal(
		t,
		[]response.Warning{{Code: "html_sanitized"}},
		response.Detail(savedHTML, savedSource, "https://app.test", false).Warnings,
	)

	ticket, err := u.PreviewTicket(ctx, owner, htmlKnowledge, 1)
	require.NoError(t, err)
	content, err := u.PrivateHTML(ctx, ticket)
	require.NoError(t, err)
	require.Contains(t, content, "safe")
	require.NotContains(t, content, "script")

	session.active = false
	_, err = u.PrivateHTML(ctx, ticket)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = u.PreviewTicket(ctx, owner, htmlKnowledge, 1)
	require.ErrorIs(t, err, domain.ErrUnauthenticated)

	session.active = true
	session.unavailable = true
	_, err = u.PrivateHTML(ctx, ticket)
	require.ErrorIs(t, err, domain.ErrUnavailable)

	session.unavailable = false
	_, _, err = u.Save(ctx, owner, htmlKnowledge, 1, "<p>new</p>")
	require.NoError(t, err)
	_, err = u.PrivateHTML(ctx, ticket)
	require.ErrorIs(t, err, domain.ErrNotFound)
	savedHTML, savedSource, err = u.Get(ctx, owner, htmlKnowledge)
	require.NoError(t, err)
	require.False(t, savedHTML.HTMLSanitized)
	require.Empty(t, response.Detail(savedHTML, savedSource, "https://app.test", false).Warnings)

	_, warning, err := u.Save(ctx, owner, htmlKnowledge, 2, `<p>new</p><script>bad()</script>`)
	require.NoError(t, err)
	require.True(t, warning)

	savedHTML, _, err = u.Get(ctx, owner, htmlKnowledge)
	require.NoError(t, err)
	require.True(t, savedHTML.HTMLSanitized)

	recent, err := u.Recent(ctx, owner)
	require.NoError(t, err)
	require.Len(t, recent, 2)
	require.Equal(t, htmlKnowledge, recent[0].ID)

	_, err = repo.Collect(ctx, time.Now().Add(time.Hour), objects.remove)
	require.NoError(t, err)
	_, source, err = u.Get(ctx, owner, id)
	require.NoError(t, err)
	require.Contains(t, source, "# new")
	require.Contains(t, objects.values, "draft-source")

	down, err := os.ReadFile("../../../migrations/000001_knowledge.down.sql")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(down))
	require.NoError(t, err)
}
