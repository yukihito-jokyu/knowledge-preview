package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/htmlsafe"
)

// 未指定の依存呼び出しは失敗させ、拒否後の保存や読み出しを検出する。
type knowledgeRepositoryStub struct {
	KnowledgeRepository
	get           func(context.Context, string, string) (domain.Knowledge, error)
	draft         func(context.Context, string, string) (domain.Draft, error)
	list          func(context.Context, string, domain.ListQuery) (domain.KnowledgePage, error)
	stage         func(context.Context, []string, func() error) error
	create        func(context.Context, domain.Draft) error
	save          func(context.Context, domain.Knowledge, int64) (domain.Knowledge, error)
	grant         func(context.Context, domain.PreviewGrant) error
	preview       func(context.Context, string) (domain.PreviewGrant, error)
	backfill      func(context.Context, string) ([]domain.Knowledge, error)
	setSearchText func(context.Context, string, string, string) error
}

func (r knowledgeRepositoryStub) Get(c context.Context, owner, id string) (domain.Knowledge, error) {
	return r.get(c, owner, id)
}

func (r knowledgeRepositoryStub) Draft(c context.Context, owner, id string) (domain.Draft, error) {
	return r.draft(c, owner, id)
}

func (r knowledgeRepositoryStub) List(
	c context.Context,
	owner string,
	query domain.ListQuery,
) (domain.KnowledgePage, error) {
	return r.list(c, owner, query)
}

func (r knowledgeRepositoryStub) SearchTextBackfill(c context.Context, owner string) ([]domain.Knowledge, error) {
	return r.backfill(c, owner)
}

func (r knowledgeRepositoryStub) SetSearchText(c context.Context, owner, id, value string) error {
	return r.setSearchText(c, owner, id, value)
}

func (r knowledgeRepositoryStub) StageObjects(c context.Context, keys []string, put func() error) error {
	return r.stage(c, keys, put)
}

func (r knowledgeRepositoryStub) CreateDraft(c context.Context, draft domain.Draft) error {
	return r.create(c, draft)
}

func (r knowledgeRepositoryStub) Save(c context.Context, k domain.Knowledge, v int64) (domain.Knowledge, error) {
	return r.save(c, k, v)
}

func (r knowledgeRepositoryStub) Grant(c context.Context, g domain.PreviewGrant) error {
	return r.grant(c, g)
}

func (r knowledgeRepositoryStub) Preview(c context.Context, hash string) (domain.PreviewGrant, error) {
	return r.preview(c, hash)
}

type sessionCheck func(context.Context, domain.Principal) (bool, error)

func (s sessionCheck) Active(c context.Context, p domain.Principal) (bool, error) { return s(c, p) }

type objectStub struct {
	KnowledgeObjects
	put func(context.Context, string, string) error
	get func(context.Context, string) (string, error)
}

func (o objectStub) Put(c context.Context, key, source string) error   { return o.put(c, key, source) }
func (o objectStub) Get(c context.Context, key string) (string, error) { return o.get(c, key) }

type sanitizerFunc func(string) (string, bool, error)

func (s sanitizerFunc) Sanitize(source string) (string, bool, error) { return s(source) }

func activeSession(context.Context, domain.Principal) (bool, error) { return true, nil }

func TestKnowledgeRejectsUnauthorizedOperations(t *testing.T) {
	principal := domain.Principal{OwnerID: "owner", SessionID: "session"}
	for _, test := range []struct {
		name      string
		principal domain.Principal
		sessions  Sessions
		want      error
	}{
		{
			name:      "所有者なし",
			principal: domain.Principal{SessionID: "session"},
			sessions:  sessionCheck(activeSession),
			want:      domain.ErrUnauthenticated,
		},
		{
			name:      "セッションIDなし",
			principal: domain.Principal{OwnerID: "owner"},
			sessions:  sessionCheck(activeSession),
			want:      domain.ErrUnauthenticated,
		},
		{
			name:      "セッション確認未接続",
			principal: principal,
			want:      domain.ErrUnauthenticated,
		},
		{
			name:      "失効済み",
			principal: principal,
			sessions:  DenySessions{},
			want:      domain.ErrUnauthenticated,
		},
		{
			name:      "セッションDB障害",
			principal: principal,
			sessions:  sessionCheck(func(context.Context, domain.Principal) (bool, error) { return false, errors.New("DB障害") }),
			want:      domain.ErrUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			u := NewKnowledgeUseCase(nil, nil, nil, test.sessions)
			c := t.Context()
			_, _, err := u.Get(c, test.principal, "id")
			require.ErrorIs(t, err, test.want)
			_, err = u.Recent(c, test.principal)
			require.ErrorIs(t, err, test.want)
			_, _, err = u.Draft(c, test.principal, "id")
			require.ErrorIs(t, err, test.want)
			_, _, err = u.Save(c, test.principal, "id", 1, "body")
			require.ErrorIs(t, err, test.want)
			_, _, err = u.Commit(c, test.principal, "id", 1, "body")
			require.ErrorIs(t, err, test.want)
			_, _, err = u.Visibility(c, test.principal, "id", 1, "unlisted")
			require.ErrorIs(t, err, test.want)
			_, err = u.PreviewTicket(c, test.principal, "id", 1)
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestKnowledgeListValidatesBeforeAndOnlyBackfillsSearches(t *testing.T) {
	var backfillCalls int

	repo := knowledgeRepositoryStub{
		list: func(_ context.Context, _ string, query domain.ListQuery) (domain.KnowledgePage, error) {
			require.Empty(t, query.Query)

			return domain.KnowledgePage{Page: 1, PageSize: 10}, nil
		},
		backfill: func(context.Context, string) ([]domain.Knowledge, error) {
			backfillCalls++

			return nil, domain.ErrUnavailable
		},
	}
	u := NewKnowledgeUseCase(repo, nil, nil, sessionCheck(activeSession))
	p := domain.Principal{OwnerID: "owner", SessionID: "session"}

	page, err := u.List(t.Context(), p, domain.ListQuery{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, page.Page)
	require.Zero(t, backfillCalls)

	_, err = u.List(
		t.Context(),
		p,
		domain.ListQuery{Query: strings.Repeat("x", domain.MaxSearchRunes+1), Page: 1, PageSize: 10},
	)
	require.ErrorIs(t, err, domain.ErrBadRequest)
	require.Zero(t, backfillCalls)

	_, err = u.List(t.Context(), p, domain.ListQuery{Query: "body", Page: 1, PageSize: 10})
	require.ErrorIs(t, err, domain.ErrUnavailable)
	require.Equal(t, 1, backfillCalls)
}

func TestKnowledgeSave(t *testing.T) {
	failure := errors.New("保存障害")
	for _, test := range []struct {
		name       string
		version    int64
		source     string
		failAt     string
		want       error
		wantEvents []string
	}{
		{
			name:       "HTMLの原文と安全化結果を保存",
			version:    1,
			source:     "<p>本文</p><script>x</script>",
			wantEvents: []string{"stage", "source", "html", "save"},
		},
		{
			name:    "古い版は保存しない",
			version: 2,
			source:  "body",
			want:    domain.ErrConflict,
		},
		{
			name:    "空本文は保存しない",
			version: 1,
			source:  " ",
			want:    domain.ErrValidation,
		},
		{
			name:    "安全化失敗",
			version: 1,
			source:  "body",
			failAt:  "sanitize",
			want:    failure,
		},
		{
			name:       "保留記録失敗",
			version:    1,
			source:     "body",
			failAt:     "stage",
			want:       failure,
			wantEvents: []string{"stage"},
		},
		{
			name:       "原文保存失敗",
			version:    1,
			source:     "body",
			failAt:     "source",
			want:       failure,
			wantEvents: []string{"stage", "source"},
		},
		{
			name:       "HTML保存失敗",
			version:    1,
			source:     "body",
			failAt:     "html",
			want:       failure,
			wantEvents: []string{"stage", "source", "html"},
		},
		{
			name:       "DB確定失敗",
			version:    1,
			source:     "body",
			failAt:     "save",
			want:       failure,
			wantEvents: []string{"stage", "source", "html", "save"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events, keys []string

			repo := knowledgeRepositoryStub{
				get: func(_ context.Context, owner, id string) (domain.Knowledge, error) {
					require.Equal(t, "owner", owner)
					require.Equal(t, "id", id)

					return domain.Knowledge{ID: id, OwnerID: owner, Format: "html", Title: "題名", Version: 1}, nil
				},
				stage: func(_ context.Context, k []string, put func() error) error {
					events = append(events, "stage")

					if test.failAt == "stage" {
						return failure
					}

					require.Len(t, k, 2)
					require.NotEqual(t, k[0], k[1])
					keys = k

					return put()
				},
				save: func(_ context.Context, k domain.Knowledge, v int64) (domain.Knowledge, error) {
					events = append(events, "save")

					require.Equal(t, int64(1), v)
					require.Equal(t, keys[0], k.SourceKey)
					require.Equal(t, keys[1], k.HTMLKey)
					require.True(t, k.HTMLSanitized)

					if test.failAt == "save" {
						return domain.Knowledge{}, failure
					}

					k.Version++

					return k, nil
				},
			}
			objects := objectStub{put: func(_ context.Context, key, source string) error {
				event := "source"
				if key == keys[1] {
					event = "html"

					require.Equal(t, "<p>本文</p>", source)
				} else {
					require.Equal(t, keys[0], key)
					require.Equal(t, test.source, source)
				}

				events = append(events, event)
				if test.failAt == event {
					return failure
				}

				return nil
			}}
			sanitizer := sanitizerFunc(func(source string) (string, bool, error) {
				require.Equal(t, test.source, source)

				if test.failAt == "sanitize" {
					return "", false, failure
				}

				return "<p>本文</p>", true, nil
			})
			u := NewKnowledgeUseCase(repo, objects, sanitizer, sessionCheck(activeSession))
			saved, warning, err := u.Save(
				t.Context(),
				domain.Principal{OwnerID: "owner", SessionID: "session"},
				"id",
				test.version,
				test.source,
			)
			require.ErrorIs(t, err, test.want)
			require.Equal(t, test.wantEvents, events)

			if test.want == nil {
				require.Equal(t, int64(2), saved.Version)
				require.True(t, warning)
			}
		})
	}
}

func TestKnowledgeUploadExtractsMetadataAndStagesOneObject(t *testing.T) {
	var saved domain.Draft

	objects := objectStub{put: func(_ context.Context, key, source string) error {
		require.True(t, strings.HasPrefix(key, "knowledge/"))
		require.Equal(t, "---\ntitle: 本文タイトル\ntags: [go, db]\n---\n本文", source)

		return nil
	}}
	repo := knowledgeRepositoryStub{
		stage: func(_ context.Context, keys []string, put func() error) error {
			require.Len(t, keys, 1)
			return put()
		},
		create: func(_ context.Context, draft domain.Draft) error {
			saved = draft
			return nil
		},
	}
	u := NewKnowledgeUseCase(repo, objects, htmlsafe.New(), sessionCheck(activeSession))
	draft, err := u.Upload(
		t.Context(),
		domain.Principal{OwnerID: "owner", SessionID: "session"},
		"ignored.md",
		"---\ntitle: 本文タイトル\ntags: [go, db]\n---\n本文",
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, saved.DraftID, draft.DraftID)
	require.Equal(t, "本文タイトル", draft.Title)
	require.Equal(t, []string{"go", "db"}, draft.Tags)
	require.Equal(t, "markdown", draft.Format)
}

func TestKnowledgeCommitRetry(t *testing.T) {
	hash := sha256.Sum256([]byte("body"))

	for _, test := range []struct {
		name    string
		version int64
		source  string
		want    error
	}{
		{
			name:    "同じ本文は同じ保存済みID",
			version: 1,
			source:  "body",
		},
		{
			name:    "本文が異なる再送",
			version: 1,
			source:  "changed",
			want:    domain.ErrConflict,
		},
		{
			name:    "版が異なる再送",
			version: 2,
			source:  "body",
			want:    domain.ErrConflict,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := knowledgeRepositoryStub{draft: func(_ context.Context, owner, id string) (domain.Draft, error) {
				require.Equal(t, "owner", owner)
				require.Equal(t, "draft", id)

				return domain.Draft{
					Knowledge:   domain.Knowledge{Version: 1},
					CommittedID: "saved",
					CommitHash:  hex.EncodeToString(hash[:]),
				}, nil
			}}
			u := NewKnowledgeUseCase(repo, nil, nil, sessionCheck(activeSession))
			id, created, err := u.Commit(
				t.Context(),
				domain.Principal{OwnerID: "owner", SessionID: "session"},
				"draft",
				test.version,
				test.source,
			)
			require.ErrorIs(t, err, test.want)
			require.False(t, created)

			if test.want == nil {
				require.Equal(t, "saved", id)
			} else {
				require.Empty(t, id)
			}
		})
	}
}

func TestKnowledgePreviewRevocation(t *testing.T) {
	for _, test := range []struct {
		name      string
		active    bool
		version   int64
		wantHTML  string
		wantError error
		wantReads int
	}{
		{
			name:      "有効なセッションと同じ版なら再取得できる",
			active:    true,
			version:   1,
			wantHTML:  "<p>本文</p>",
			wantReads: 2,
		},
		{
			name:      "ログアウト後は同じURLでも拒否する",
			active:    false,
			version:   1,
			wantError: domain.ErrNotFound,
			wantReads: 1,
		},
		{
			name:      "本文更新後は旧版URLを拒否する",
			active:    true,
			version:   2,
			wantError: domain.ErrNotFound,
			wantReads: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			principal := domain.Principal{OwnerID: "owner", SessionID: "session"}

			var grant domain.PreviewGrant

			active := true
			checks := 0
			reads := 0
			version := int64(1)
			repo := knowledgeRepositoryStub{
				get: func(_ context.Context, owner, id string) (domain.Knowledge, error) {
					require.Equal(t, principal.OwnerID, owner)
					require.Equal(t, "id", id)

					return domain.Knowledge{Format: "html", Version: version, HTMLKey: "clean"}, nil
				},
				grant: func(_ context.Context, g domain.PreviewGrant) error { grant = g; return nil },
				preview: func(_ context.Context, hash string) (domain.PreviewGrant, error) {
					require.Equal(t, grant.Hash, hash)
					return grant, nil
				},
			}
			sessions := sessionCheck(func(_ context.Context, p domain.Principal) (bool, error) {
				require.Equal(t, principal, p)

				checks++

				return active, nil
			})
			objects := objectStub{get: func(_ context.Context, key string) (string, error) {
				require.Equal(t, "clean", key)

				reads++

				return "<p>本文</p>", nil
			}}
			u := NewKnowledgeUseCase(repo, objects, nil, sessions)
			token, err := u.PreviewTicket(t.Context(), principal, "id", 1)
			require.NoError(t, err)
			require.Len(t, token, 43)
			require.NotEqual(t, token, grant.Hash)
			require.Equal(t, principal.SessionID, grant.SessionID)
			require.Equal(t, int64(1), grant.Version)
			html, err := u.PrivateHTML(t.Context(), token)
			require.NoError(t, err)
			require.Equal(t, "<p>本文</p>", html)

			active = test.active
			version = test.version
			html, err = u.PrivateHTML(t.Context(), token)
			require.ErrorIs(t, err, test.wantError)
			require.Equal(t, test.wantHTML, html)
			require.Equal(t, 3, checks)
			require.Equal(t, test.wantReads, reads)
		})
	}
}

func TestKnowledgePrivateHTMLRejectsInvalidTicket(t *testing.T) {
	for _, test := range []struct {
		name  string
		token string
	}{
		{
			name:  "空のチケット",
			token: "",
		},
		{
			name:  "規定より1文字短い",
			token: strings.Repeat("x", 42),
		},
		{
			name:  "規定より1文字長い",
			token: strings.Repeat("x", 44),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			u := NewKnowledgeUseCase(nil, nil, nil, sessionCheck(activeSession))
			html, err := u.PrivateHTML(t.Context(), test.token)
			require.ErrorIs(t, err, domain.ErrNotFound)
			require.Empty(t, html)
		})
	}
}
