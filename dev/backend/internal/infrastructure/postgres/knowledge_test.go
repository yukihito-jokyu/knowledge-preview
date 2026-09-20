package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

	for _, name := range []string{"000001_knowledge.up.sql", "000002_auth.up.sql", "000003_knowledge_library.up.sql"} {
		migration, err := os.ReadFile("../../../migrations/" + name)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, string(migration))
		require.NoError(t, err)
	}

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

	for _, name := range []string{"000003_knowledge_library.down.sql", "000002_auth.down.sql", "000001_knowledge.down.sql"} {
		down, err := os.ReadFile("../../../migrations/" + name)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, string(down))
		require.NoError(t, err)
	}
}

func TestKnowledgeLibraryIntegration(t *testing.T) {
	database := os.Getenv("KNOWLEDGE_TEST_DATABASE_URL")
	if database == "" {
		t.Skip("set KNOWLEDGE_TEST_DATABASE_URL to a disposable local PostgreSQL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, database)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	schema := "knowledge_library_test_" + time.Now().Format("20060102150405")
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

	for _, name := range []string{"000001_knowledge.up.sql", "000002_auth.up.sql", "000003_knowledge_library.up.sql"} {
		migration, readErr := os.ReadFile("../../../migrations/" + name)
		require.NoError(t, readErr)

		_, err = testPool.Exec(ctx, string(migration))
		require.NoError(t, err)

		if name == "000002_auth.up.sql" {
			_, err = testPool.Exec(
				ctx,
				`INSERT INTO knowledge_folders(id,owner_id,name) VALUES
				 ('33333333-3333-4333-8333-333333333331','legacy-owner','same'),
				 ('33333333-3333-4333-8333-333333333332','legacy-owner','same');
				INSERT INTO knowledge_objects(object_key) VALUES('legacy-source');
				INSERT INTO knowledge(id,owner_id,title,format,source_key) VALUES
				 ('44444444-4444-4444-8444-444444444444','owner-a','legacy','markdown','legacy-source')`,
			)
			require.NoError(t, err)
		}
	}

	objects := &memoryObjects{values: map[string]string{}}
	objects.values["legacy-source"] = "legacy body searchable"
	objects.values["backfill-failure-source"] = "recovered body searchable"
	session := &sessions{active: true}
	u := usecase.NewKnowledgeUseCase(postgres.NewKnowledgeRepository(testPool), objects, htmlsafe.New(), session)
	owner := domain.Principal{OwnerID: "owner-a", SessionID: "session-a"}
	other := domain.Principal{OwnerID: "owner-b", SessionID: "session-a"}

	var legacyFolders int

	err = testPool.QueryRow(ctx, `SELECT count(*) FROM knowledge_folders WHERE owner_id='legacy-owner' AND name='same'`).
		Scan(&legacyFolders)
	require.NoError(t, err)
	require.Equal(t, 2, legacyFolders)

	legacyPage, err := u.List(ctx, owner, domain.ListQuery{Query: "legacy body", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, legacyPage.Items, 1)
	require.Equal(t, "44444444-4444-4444-8444-444444444444", legacyPage.Items[0].ID)

	_, err = testPool.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key) VALUES('backfill-failure-source'); INSERT INTO knowledge(id,owner_id,title,format,source_key) VALUES('44444444-4444-4444-8444-444444444445','owner-a','backfill failure','markdown','backfill-failure-source')`,
	)
	require.NoError(t, err)

	objects.fail = true
	page, err := u.List(ctx, owner, domain.ListQuery{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.NotEmpty(t, page.Items)

	_, err = u.List(
		ctx,
		owner,
		domain.ListQuery{Query: strings.Repeat("x", domain.MaxSearchRunes+1), Page: 1, PageSize: 10},
	)
	require.ErrorIs(t, err, domain.ErrBadRequest)

	objects.fail = false
	page, err = u.List(ctx, owner, domain.ListQuery{Query: "searchable", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.ElementsMatch(
		t,
		[]string{
			"44444444-4444-4444-8444-444444444444",
			"44444444-4444-4444-8444-444444444445",
		},
		[]string{page.Items[0].ID, page.Items[1].ID},
	)

	root, err := u.CreateFolder(ctx, owner, "Root", nil)
	require.NoError(t, err)
	_, err = u.CreateFolder(ctx, owner, "Root", nil)
	require.ErrorIs(t, err, domain.ErrConflict)
	child, err := u.CreateFolder(ctx, owner, "Child", &root.ID)
	require.NoError(t, err)
	require.Equal(t, root.ID, *child.ParentID)
	require.EqualValues(t, 1, child.Version)
	_, err = u.UpdateFolder(ctx, owner, root.ID, "Root", &child.ID, true, root.Version)
	require.ErrorIs(t, err, domain.ErrValidation)
	_, err = u.UpdateFolder(ctx, owner, root.ID, "Root 2", nil, false, root.Version+1)
	require.ErrorIs(t, err, domain.ErrConflict)
	branchA, err := u.CreateFolder(ctx, owner, "A", nil)
	require.NoError(t, err)
	branchC, err := u.CreateFolder(ctx, owner, "C", nil)
	require.NoError(t, err)
	branchB, err := u.CreateFolder(ctx, owner, "B", &branchA.ID)
	require.NoError(t, err)
	branchD, err := u.CreateFolder(ctx, owner, "D", &branchC.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)

	go func() {
		<-start

		_, updateErr := u.UpdateFolder(ctx, owner, branchA.ID, "A", &branchD.ID, true, branchA.Version)
		results <- updateErr
	}()
	go func() {
		<-start

		_, updateErr := u.UpdateFolder(ctx, owner, branchC.ID, "C", &branchB.ID, true, branchC.Version)
		results <- updateErr
	}()

	close(start)

	first, second := <-results, <-results
	require.NotEqual(t, first == nil, second == nil)
	require.True(t, (first == nil && errors.Is(second, domain.ErrValidation)) ||
		(first != nil && errors.Is(first, domain.ErrValidation)))

	var cycle bool

	err = testPool.QueryRow(ctx, `WITH RECURSIVE paths(id,ancestor) AS (
		SELECT id,parent_id FROM knowledge_folders WHERE owner_id='owner-a'
		UNION
		SELECT paths.id,f.parent_id FROM paths JOIN knowledge_folders f ON f.id=paths.ancestor WHERE paths.ancestor IS NOT NULL
	) SELECT EXISTS(SELECT 1 FROM paths WHERE id=ancestor)`).Scan(&cycle)
	require.NoError(t, err)
	require.False(t, cycle)

	draft, err := u.Upload(ctx, owner, "guide.md", "---\ntags: [go, db]\n---\nsearchable body", &child.ID)
	require.NoError(t, err)
	knowledgeID, created, err := u.Commit(ctx, owner, draft.DraftID, 1, "---\ntags: [go, db]\n---\nsearchable body")
	require.NoError(t, err)
	require.True(t, created)

	before, _, err := u.Get(ctx, owner, knowledgeID)
	require.NoError(t, err)
	require.NotNil(t, before.Folder)
	require.Equal(t, child.ID, before.Folder.ID)
	require.Equal(t, child.Name, before.Folder.Name)
	require.Equal(t, root.ID, *before.Folder.ParentID)
	require.Equal(t, child.Version, before.Folder.Version)

	page, err = u.List(ctx, owner, domain.ListQuery{Query: "searchable", Tags: []string{"go"}, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, page.Total)
	require.Equal(t, knowledgeID, page.Items[0].ID)
	require.NotNil(t, page.Items[0].Folder)
	require.Equal(t, child.ID, page.Items[0].Folder.ID)
	require.Equal(t, root.ID, *page.Items[0].Folder.ParentID)
	require.Equal(t, child.Version, page.Items[0].Folder.Version)
	childPage, err := u.List(ctx, owner, domain.ListQuery{FolderID: &child.ID, Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 1, childPage.Total)
	require.Equal(t, 1, childPage.Page)
	require.Equal(t, 2, childPage.PageSize)
	require.Len(t, childPage.Items, 1)
	childPage, err = u.List(ctx, owner, domain.ListQuery{FolderID: &child.ID, Page: 2, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 1, childPage.Total)
	require.Equal(t, 2, childPage.Page)
	require.Empty(t, childPage.Items)

	otherFolderPage, err := u.List(ctx, other, domain.ListQuery{FolderID: &child.ID, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Zero(t, otherFolderPage.Total)
	require.Empty(t, otherFolderPage.Items)

	page, err = u.List(ctx, other, domain.ListQuery{Query: "searchable", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Empty(t, page.Items)

	rootFolderID := ""
	rootPage, err := u.List(ctx, owner, domain.ListQuery{FolderID: &rootFolderID, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, rootPage.Items, 2)
	require.ElementsMatch(
		t,
		[]string{
			"44444444-4444-4444-8444-444444444444",
			"44444444-4444-4444-8444-444444444445",
		},
		[]string{rootPage.Items[0].ID, rootPage.Items[1].ID},
	)

	for _, item := range rootPage.Items {
		require.Nil(t, item.Folder)
	}

	after, _, err := u.Get(ctx, owner, knowledgeID)
	require.NoError(t, err)
	require.Equal(t, before.UpdatedAt, after.UpdatedAt)

	err = u.DeleteFolder(ctx, owner, child.ID, child.Version)
	require.ErrorIs(t, err, domain.ErrConflict)
	_, err = u.MoveKnowledge(ctx, owner, knowledgeID, nil, after.Version)
	require.NoError(t, err)
	folders, err := postgres.NewKnowledgeRepository(testPool).Folders(ctx, owner.OwnerID)
	require.NoError(t, err)
	require.Len(t, folders, 6)

	var childFolder domain.Folder

	for _, folder := range folders {
		if folder.ID == child.ID {
			childFolder = folder
		}
	}

	require.Equal(t, child.ID, childFolder.ID)
	require.Equal(t, child.Version, childFolder.Version)
	err = u.DeleteFolder(ctx, owner, child.ID, child.Version)
	require.ErrorIs(t, err, domain.ErrConflict)

	largeSource := strings.Repeat("本文 ", (domain.MaxSourceBytes-len("\ntail-search-term"))/len("本文 "))
	largeSource += strings.Repeat("x", domain.MaxSourceBytes-len(largeSource)-len("\ntail-search-term"))
	largeSource += "\ntail-search-term"
	largeDraft, err := u.Upload(ctx, owner, "large.md", largeSource, nil)
	require.NoError(t, err)
	largeID, created, err := u.Commit(ctx, owner, largeDraft.DraftID, 1, largeSource)
	require.NoError(t, err)
	require.True(t, created)

	_, _, err = u.Save(ctx, owner, largeID, 1, largeSource)
	require.NoError(t, err)
	largePage, err := u.List(ctx, owner, domain.ListQuery{Query: "tail-search-term", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, largePage.Items, 1)
	require.Equal(t, largeID, largePage.Items[0].ID)
	largePage, err = u.List(ctx, owner, domain.ListQuery{Query: "本文 tail-search-term", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, largePage.Items, 1)
	require.Equal(t, largeID, largePage.Items[0].ID)
	largePage, err = u.List(ctx, owner, domain.ListQuery{Query: "本文 -tail-search-term", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Empty(t, largePage.Items)

	longSource := strings.Repeat("padding ", 13000) + "alpha intervening beta"
	longDraft, err := u.Upload(ctx, owner, "long.md", longSource, nil)
	require.NoError(t, err)
	longID, created, err := u.Commit(ctx, owner, longDraft.DraftID, 1, longSource)
	require.NoError(t, err)
	require.True(t, created)

	longPage, err := u.List(ctx, owner, domain.ListQuery{Query: "alpha beta", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, longPage.Items, 1)
	require.Equal(t, longID, longPage.Items[0].ID)
	longPage, err = u.List(ctx, owner, domain.ListQuery{Query: "alpha or missing", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, longPage.Items, 1)
	require.Equal(t, longID, longPage.Items[0].ID)
	longPage, err = u.List(ctx, owner, domain.ListQuery{Query: "padding -beta", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Empty(t, longPage.Items)
	longPage, err = u.List(ctx, owner, domain.ListQuery{Query: `"alpha intervening beta"`, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, longPage.Items, 1)
	require.Equal(t, longID, longPage.Items[0].ID)
	longPage, err = u.List(ctx, owner, domain.ListQuery{Query: `"alpha beta"`, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, longPage.Items, 1)
	require.Equal(t, longID, longPage.Items[0].ID)
	longPage, err = u.List(ctx, owner, domain.ListQuery{Query: `"beta alpha"`, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, longPage.Items, 1)
	require.Equal(t, longID, longPage.Items[0].ID)
	longPage, err = u.List(ctx, owner, domain.ListQuery{Query: `"alpha intervening beta`, Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, longPage.Items, 1)
	require.Equal(t, longID, longPage.Items[0].ID)

	spaceSource := "alpha" + strings.Repeat(" ", 20000) + "beta"
	spaceDraft, err := u.Upload(ctx, owner, "space.md", spaceSource, nil)
	require.NoError(t, err)
	spaceID, created, err := u.Commit(ctx, owner, spaceDraft.DraftID, 1, spaceSource)
	require.NoError(t, err)
	require.True(t, created)

	spacePage, err := u.List(ctx, owner, domain.ListQuery{Query: `"alpha beta"`, Page: 1, PageSize: 100})
	require.NoError(t, err)

	var spaceFound bool
	for _, item := range spacePage.Items {
		spaceFound = spaceFound || item.ID == spaceID
	}

	require.True(t, spaceFound)

	_, err = testPool.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key) VALUES('boundary-source'); INSERT INTO knowledge(id,owner_id,title,format,source_key,search_text,search_text_ready) VALUES('55555555-5555-4555-8555-555555555555','owner-a','boundary','markdown','boundary-source','alphabeta',true)`,
	)
	require.NoError(t, err)
	boundaryPage, err := u.List(ctx, owner, domain.ListQuery{Query: "alpha", Page: 1, PageSize: 100})
	require.NoError(t, err)

	for _, item := range boundaryPage.Items {
		require.NotEqual(t, "55555555-5555-4555-8555-555555555555", item.ID)
	}

	_, err = testPool.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key) VALUES('sentinel-source'),('number-source'); INSERT INTO knowledge(id,owner_id,title,format,source_key,search_text,search_text_ready) VALUES
			('55555555-5555-4555-8555-555555555557','owner-a','sentinel','markdown','sentinel-source','kpqphrase1',true),
			('55555555-5555-4555-8555-555555555558','owner-a','number','markdown','number-source','123',true)`,
	)
	require.NoError(t, err)

	sentinelPage, err := u.List(ctx, owner, domain.ListQuery{Query: `"alpha beta" kpqphrase1`, Page: 1, PageSize: 100})
	require.NoError(t, err)

	for _, item := range sentinelPage.Items {
		require.NotEqual(t, "55555555-5555-4555-8555-555555555557", item.ID)
	}

	numberPage, err := u.List(ctx, owner, domain.ListQuery{Query: "-123", Page: 1, PageSize: 100})
	require.NoError(t, err)

	for _, item := range numberPage.Items {
		require.NotEqual(t, "55555555-5555-4555-8555-555555555558", item.ID)
	}

	_, err = testPool.Exec(
		ctx,
		`INSERT INTO knowledge_objects(object_key) VALUES('parser-source'); INSERT INTO knowledge(id,owner_id,title,format,source_key,search_text,search_text_ready) VALUES('55555555-5555-4555-8555-555555555556','owner-a','parser','markdown','parser-source','example.com support@example.com',true)`,
	)
	require.NoError(t, err)
	parserPage, err := u.List(ctx, owner, domain.ListQuery{Query: "example.com", Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.NotEmpty(t, parserPage.Items)
	require.Equal(t, "55555555-5555-4555-8555-555555555556", parserPage.Items[0].ID)
	parserPage, err = u.List(ctx, owner, domain.ListQuery{Query: "support@example.com", Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.NotEmpty(t, parserPage.Items)
	require.Equal(t, "55555555-5555-4555-8555-555555555556", parserPage.Items[0].ID)

	var distinctSource strings.Builder
	for i := 0; i < 70000; i++ {
		fmt.Fprintf(&distinctSource, "distinct%06d ", i)
	}

	distinctDraft, err := u.Upload(ctx, owner, "distinct.md", distinctSource.String(), nil)
	require.NoError(t, err)
	distinctID, created, err := u.Commit(ctx, owner, distinctDraft.DraftID, 1, distinctSource.String())
	require.NoError(t, err)
	require.True(t, created)

	distinctPage, err := u.List(ctx, owner, domain.ListQuery{Query: "distinct069999", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, distinctPage.Items, 1)
	require.Equal(t, distinctID, distinctPage.Items[0].ID)

	for i := 0; i < 101; i++ {
		objectKey := fmt.Sprintf("backfill-%03d", i)
		id := fmt.Sprintf("66666666-6666-4666-8666-%012d", i+1)

		value := "bulk filler"
		if i == 100 {
			value = "backfill-last-term"
		}

		objects.values[objectKey] = value
		_, err = testPool.Exec(ctx, `INSERT INTO knowledge_objects(object_key) VALUES($1)`, objectKey)
		require.NoError(t, err)
		_, err = testPool.Exec(
			ctx,
			`INSERT INTO knowledge(id,owner_id,title,format,source_key) VALUES($1,'owner-a','bulk','markdown',$2)`,
			id,
			objectKey,
		)
		require.NoError(t, err)
	}

	_, err = u.List(ctx, owner, domain.ListQuery{Query: "backfill-last-term", Page: 1, PageSize: 10})
	require.ErrorIs(t, err, domain.ErrUnavailable)
	lastPage, err := u.List(ctx, owner, domain.ListQuery{Query: "backfill-last-term", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, lastPage.Items, 1)
	require.Equal(t, "66666666-6666-4666-8666-000000000101", lastPage.Items[0].ID)

	for _, name := range []string{"000003_knowledge_library.down.sql", "000002_auth.down.sql", "000001_knowledge.down.sql"} {
		migration, readErr := os.ReadFile("../../../migrations/" + name)
		require.NoError(t, readErr)

		_, err = testPool.Exec(ctx, string(migration))
		require.NoError(t, err)
	}
}
