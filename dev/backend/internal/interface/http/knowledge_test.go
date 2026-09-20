package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/htmlsafe"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type handlerRepository struct{ usecase.KnowledgeRepository }

func (handlerRepository) Get(_ context.Context, owner, id string) (domain.Knowledge, error) {
	if owner != "owner" {
		return domain.Knowledge{}, domain.ErrNotFound
	}

	return domain.Knowledge{
		ID:             id,
		OwnerID:        owner,
		Title:          "title",
		Format:         "markdown",
		Tags:           []string{},
		LearningStatus: "unlearned",
		Version:        1,
		Visibility:     "private",
		SourceKey:      "source",
	}, nil
}

type handlerObjects struct {
	putErr error
}

func (handlerObjects) Get(context.Context, string) (string, error) { return "# body", nil }
func (o handlerObjects) Put(context.Context, string, string) error { return o.putErr }

type handlerSessions struct{}

func (handlerSessions) Active(context.Context, domain.Principal) (bool, error) { return true, nil }

type libraryHandlerRepository struct {
	handlerRepository
	draft     domain.Draft
	stageErr  error
	createErr error
	listQuery domain.ListQuery
	listPage  domain.KnowledgePage
}

func (r *libraryHandlerRepository) List(
	_ context.Context,
	_ string,
	query domain.ListQuery,
) (domain.KnowledgePage, error) {
	r.listQuery = query
	if r.listPage.Items != nil {
		return r.listPage, nil
	}

	return domain.KnowledgePage{
		Page:     1,
		PageSize: 10,
		Items: []domain.Knowledge{
			{ID: "11111111-1111-4111-8111-111111111111", Title: "title", Format: "markdown", Tags: []string{}},
		},
		Total: 1,
	}, nil
}

func (r *libraryHandlerRepository) StageObjects(_ context.Context, _ []string, put func() error) error {
	if r.stageErr != nil {
		return r.stageErr
	}

	return put()
}

func (r *libraryHandlerRepository) CreateDraft(_ context.Context, draft domain.Draft) error {
	if r.createErr != nil {
		return r.createErr
	}

	r.draft = draft

	return nil
}

func (*libraryHandlerRepository) Folders(context.Context, string) ([]domain.Folder, error) {
	return []domain.Folder{}, nil
}

func (*libraryHandlerRepository) CreateFolder(
	_ context.Context,
	_ string,
	name string,
	parentID *string,
) (domain.Folder, error) {
	return domain.Folder{
		ID:       "22222222-2222-4222-8222-222222222222",
		Name:     name,
		ParentID: parentID,
		Version:  1,
	}, nil
}

func (*libraryHandlerRepository) UpdateFolder(
	_ context.Context,
	_ string,
	id, name string,
	parentID *string,
	_ bool,
	version int64,
) (domain.Folder, error) {
	return domain.Folder{ID: id, Name: name, ParentID: parentID, Version: version + 1}, nil
}

func (*libraryHandlerRepository) DeleteFolder(context.Context, string, string, int64) error {
	return nil
}

func (*libraryHandlerRepository) MoveKnowledge(
	_ context.Context,
	_ string,
	id string,
	folderID *string,
	version int64,
) (domain.Knowledge, error) {
	var folder *domain.Folder
	if folderID != nil {
		folder = &domain.Folder{ID: *folderID, Version: 1}
	}

	return domain.Knowledge{
		ID:         id,
		Title:      "title",
		Format:     "markdown",
		Tags:       []string{},
		Folder:     folder,
		Version:    version + 1,
		Visibility: "private",
	}, nil
}

func TestKnowledgeUploadAndListHTTP(t *testing.T) {
	repo := &libraryHandlerRepository{}
	u := usecase.NewKnowledgeUseCase(repo, handlerObjects{}, htmlsafe.New(), handlerSessions{})
	router := NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	Authenticate := func(*gin.Context) (domain.Principal, error) {
		return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
	}
	RegisterKnowledge(router, u, Authenticate, "https://app.test", "https://preview.test")

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "guide.md")
	require.NoError(t, err)
	_, err = part.Write([]byte("# body"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "https://app.test/api/v1/knowledge-drafts", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "https://app.test")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "/knowledge-drafts/")

	request = httptest.NewRequest(http.MethodGet, "https://app.test/api/v1/knowledge?page=1&pageSize=1", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"total":1`)
	require.Equal(t, 1, repo.listQuery.PageSize)
}

func TestKnowledgeListHTTPFolderDTO(t *testing.T) {
	const (
		knowledgeID = "11111111-1111-4111-8111-111111111111"
		folderID    = "22222222-2222-4222-8222-222222222222"
		parentID    = "33333333-3333-4333-8333-333333333333"
	)

	folderParentID := parentID

	repo := &libraryHandlerRepository{listPage: domain.KnowledgePage{
		Items: []domain.Knowledge{
			{
				ID:      knowledgeID,
				Title:   "nested",
				Format:  "markdown",
				Tags:    []string{},
				Folder:  &domain.Folder{ID: folderID, Name: "child", ParentID: &folderParentID, Version: 4},
				Version: 2,
			},
			{
				ID:      "44444444-4444-4444-8444-444444444444",
				Title:   "root",
				Format:  "markdown",
				Tags:    []string{},
				Folder:  nil,
				Version: 1,
			},
		},
		Total:    2,
		Page:     1,
		PageSize: 10,
	}}
	u := usecase.NewKnowledgeUseCase(repo, handlerObjects{}, htmlsafe.New(), handlerSessions{})
	router := NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	RegisterKnowledge(router, u, func(*gin.Context) (domain.Principal, error) {
		return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
	}, "https://app.test", "https://preview.test")

	request := httptest.NewRequest(http.MethodGet, "https://app.test/api/v1/knowledge?page=1&pageSize=10", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var page struct {
		Items []struct {
			Folder *struct {
				ID       string  `json:"id"`
				Name     string  `json:"name"`
				ParentID *string `json:"parentId"`
				Version  int64   `json:"version"`
			} `json:"folder"`
		} `json:"items"`
		Total    int  `json:"total"`
		Page     int  `json:"page"`
		PageSize int  `json:"pageSize"`
		HasNext  bool `json:"hasNext"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &page))
	require.Equal(t, 2, page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 10, page.PageSize)
	require.False(t, page.HasNext)
	require.Len(t, page.Items, 2)
	require.Equal(t, folderID, page.Items[0].Folder.ID)
	require.Equal(t, "child", page.Items[0].Folder.Name)
	require.NotNil(t, page.Items[0].Folder.ParentID)
	require.Equal(t, parentID, *page.Items[0].Folder.ParentID)
	require.EqualValues(t, 4, page.Items[0].Folder.Version)
	require.Nil(t, page.Items[1].Folder)
}

func TestKnowledgeHTTPUsesCanonicalRoutesAndQuery(t *testing.T) {
	const (
		knowledgeID = "11111111-1111-4111-8111-111111111111"
		folderID    = "22222222-2222-4222-8222-222222222222"
	)

	repo := &libraryHandlerRepository{}
	u := usecase.NewKnowledgeUseCase(repo, handlerObjects{}, htmlsafe.New(), handlerSessions{})
	router := NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	RegisterKnowledge(router, u, func(*gin.Context) (domain.Principal, error) {
		return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
	}, "https://app.test", "https://preview.test")

	for _, test := range []struct {
		name, method, path, body string
		want                     int
	}{
		{name: "list query", method: http.MethodGet, path: "/api/v1/knowledge?query=term&page=1&pageSize=10", want: http.StatusOK},
		{name: "folders", method: http.MethodGet, path: "/api/v1/folders", want: http.StatusOK},
		{name: "create folder", method: http.MethodPost, path: "/api/v1/folders", body: `{"name":"parent"}`, want: http.StatusCreated},
		{name: "update folder", method: http.MethodPatch, path: "/api/v1/folders/" + folderID, body: `{"name":"renamed","version":1}`, want: http.StatusOK},
		{name: "delete folder", method: http.MethodDelete, path: "/api/v1/folders/" + folderID + "?version=1", want: http.StatusNoContent},
		{name: "move knowledge", method: http.MethodPut, path: "/api/v1/knowledge/" + knowledgeID + "/folder", body: `{"folderId":null,"version":1}`, want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "https://app.test"+test.path, strings.NewReader(test.body))
			if test.method != http.MethodGet {
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Origin", "https://app.test")
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.want, recorder.Code, recorder.Body.String())
		})
	}

	require.Equal(t, "term", repo.listQuery.Query)

	request := httptest.NewRequest(
		http.MethodGet,
		"https://app.test/api/v1/knowledge?q=ignored&page=1&pageSize=10",
		nil,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Empty(t, repo.listQuery.Query)

	for _, test := range []struct {
		name, method, path string
	}{
		{name: "patch move alias", method: http.MethodPatch, path: "/api/v1/knowledge/" + knowledgeID + "/folder"},
		{name: "move alias", method: http.MethodPut, path: "/api/v1/knowledge/" + knowledgeID + "/move"},
		{name: "knowledge folders alias", method: http.MethodGet, path: "/api/v1/knowledge-folders"},
		{name: "nested folders alias", method: http.MethodPost, path: "/api/v1/knowledge/folders"},
		{name: "put folder alias", method: http.MethodPut, path: "/api/v1/folders/" + folderID},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				test.method,
				"https://app.test"+test.path,
				strings.NewReader(`{"name":"alias","version":1}`),
			)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "https://app.test")

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
		})
	}
}

type uploadPart struct {
	field, filename, contentType, source string
}

func multipartRequest(t *testing.T, parts ...uploadPart) *http.Request {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	for _, upload := range parts {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+upload.field+`"; filename="`+upload.filename+`"`)
		header.Set("Content-Type", upload.contentType)
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write([]byte(upload.source))
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "https://app.test/api/v1/knowledge-drafts", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "https://app.test")

	return request
}

func multipartUploadRequest(t *testing.T, filename, contentType, source string) *http.Request {
	t.Helper()

	return multipartRequest(t, uploadPart{
		field:       "file",
		filename:    filename,
		contentType: contentType,
		source:      source,
	})
}

func TestKnowledgeUploadContentLengthAndMIME(t *testing.T) {
	for _, test := range []struct {
		name, filename, contentType string
		want                        int
	}{
		{name: "markdown", filename: "guide.md", contentType: "text/markdown", want: http.StatusCreated},
		{name: "plain browser markdown", filename: "guide.md", contentType: "text/plain; charset=utf-8", want: http.StatusCreated},
		{name: "octet stream", filename: "guide.md", contentType: "application/octet-stream", want: http.StatusCreated},
		{name: "html", filename: "guide.html", contentType: "text/html", want: http.StatusCreated},
		{name: "explicit mismatch", filename: "guide.md", contentType: "application/pdf", want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &libraryHandlerRepository{}
			u := usecase.NewKnowledgeUseCase(repo, handlerObjects{}, htmlsafe.New(), handlerSessions{})
			router := NewRouter(
				usecase.NewReadinessUseCase(fakeReadinessChecker{}),
				slog.New(slog.NewTextHandler(io.Discard, nil)),
			)
			RegisterKnowledge(router, u, func(*gin.Context) (domain.Principal, error) {
				return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
			}, "https://app.test", "https://preview.test")

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, multipartUploadRequest(t, test.filename, test.contentType, "# body"))
			require.Equal(t, test.want, recorder.Code, recorder.Body.String())
		})
	}

	request := multipartUploadRequest(t, "guide.md", "text/markdown", strings.Repeat("x", domain.MaxSourceBytes))
	request.ContentLength = int64(domain.MaxSourceBytes + 1*1024*1024 + 1)
	reader := &countingReader{}
	request.Body = io.NopCloser(reader)
	repo := &libraryHandlerRepository{}
	u := usecase.NewKnowledgeUseCase(repo, handlerObjects{}, htmlsafe.New(), handlerSessions{})
	router := NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	RegisterKnowledge(router, u, func(*gin.Context) (domain.Principal, error) {
		return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
	}, "https://app.test", "https://preview.test")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Equal(t, 0, reader.reads)

	repo = &libraryHandlerRepository{}
	u = usecase.NewKnowledgeUseCase(repo, handlerObjects{}, htmlsafe.New(), handlerSessions{})
	router = NewRouter(
		usecase.NewReadinessUseCase(fakeReadinessChecker{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	RegisterKnowledge(router, u, func(*gin.Context) (domain.Principal, error) {
		return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
	}, "https://app.test", "https://preview.test")

	recorder = httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		multipartUploadRequest(t, "max.md", "application/octet-stream", strings.Repeat("x", domain.MaxSourceBytes)),
	)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

	recorder = httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		multipartUploadRequest(t, "over.md", "text/markdown", strings.Repeat("x", domain.MaxSourceBytes+1)),
	)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code, recorder.Body.String())
}

func TestKnowledgeUploadBoundaryRejectsInvalidInputAndLeavesNoDraft(t *testing.T) {
	for _, test := range []struct {
		name             string
		parts            []uploadPart
		objectError      error
		createDraftError error
		unknownLength    bool
		want             int
	}{
		{
			name:  "empty file",
			parts: []uploadPart{{field: "file", filename: "empty.md", contentType: "text/markdown"}},
			want:  http.StatusUnprocessableEntity,
		},
		{
			name: "multiple files",
			parts: []uploadPart{
				{field: "file", filename: "one.md", contentType: "text/markdown", source: "one"},
				{field: "file", filename: "two.md", contentType: "text/markdown", source: "two"},
			},
			want: http.StatusBadRequest,
		},
		{
			name:  "invalid file field",
			parts: []uploadPart{{field: "document", filename: "guide.md", contentType: "text/markdown", source: "body"}},
			want:  http.StatusBadRequest,
		},
		{
			name:  "unsupported extension",
			parts: []uploadPart{{field: "file", filename: "guide.txt", contentType: "text/plain", source: "body"}},
			want:  http.StatusBadRequest,
		},
		{
			name:  "invalid UTF-8",
			parts: []uploadPart{{field: "file", filename: "guide.md", contentType: "text/markdown", source: string([]byte{0xff, 'x'})}},
			want:  http.StatusUnprocessableEntity,
		},
		{
			name:  "invalid front matter",
			parts: []uploadPart{{field: "file", filename: "guide.md", contentType: "text/markdown", source: "---\ntitle: [\n---\nbody"}},
			want:  http.StatusUnprocessableEntity,
		},
		{
			name:  "empty after HTML sanitization",
			parts: []uploadPart{{field: "file", filename: "guide.html", contentType: "text/html", source: "<script>alert(1)</script>"}},
			want:  http.StatusUnprocessableEntity,
		},
		{
			name:          "unknown content length over limit",
			parts:         []uploadPart{{field: "file", filename: "large.md", contentType: "text/markdown", source: strings.Repeat("x", domain.MaxSourceBytes+1)}},
			unknownLength: true,
			want:          http.StatusRequestEntityTooLarge,
		},
		{
			name:        "object storage failure",
			parts:       []uploadPart{{field: "file", filename: "guide.md", contentType: "text/markdown", source: "body"}},
			objectError: domain.ErrUnavailable,
			want:        http.StatusServiceUnavailable,
		},
		{
			name:             "draft persistence failure",
			parts:            []uploadPart{{field: "file", filename: "guide.md", contentType: "text/markdown", source: "body"}},
			createDraftError: domain.ErrUnavailable,
			want:             http.StatusServiceUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &libraryHandlerRepository{createErr: test.createDraftError}
			u := usecase.NewKnowledgeUseCase(
				repo,
				handlerObjects{putErr: test.objectError},
				htmlsafe.New(),
				handlerSessions{},
			)
			router := NewRouter(
				usecase.NewReadinessUseCase(fakeReadinessChecker{}),
				slog.New(slog.NewTextHandler(io.Discard, nil)),
			)
			RegisterKnowledge(router, u, func(*gin.Context) (domain.Principal, error) {
				return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
			}, "https://app.test", "https://preview.test")

			request := multipartRequest(t, test.parts...)
			if test.unknownLength {
				request.ContentLength = -1
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.want, recorder.Code, recorder.Body.String())
			require.Empty(t, repo.draft.DraftID)
		})
	}
}

type countingReader struct{ reads int }

func (r *countingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

func TestKnowledgeHTTPBoundary(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"

	u := usecase.NewKnowledgeUseCase(handlerRepository{}, handlerObjects{}, htmlsafe.New(), handlerSessions{})

	for _, test := range []struct {
		name, method, path, body, origin, host string
		authenticated                          bool
		want                                   int
	}{
		{
			name:   "no authentication",
			method: "GET",
			path:   "/api/v1/knowledge/" + id,
			want:   401,
		},
		{
			name:          "detail",
			method:        "GET",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			want:          200,
		},
		{
			name:          "bad ID",
			method:        "GET",
			path:          "/api/v1/knowledge/not-an-id",
			authenticated: true,
			want:          400,
		},
		{
			name:          "cross origin",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://evil.test",
			body:          `{"version":1,"source":"new"}`,
			want:          403,
		},
		{
			name:          "unknown field",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":"new","ownerId":"other"}`,
			want:          400,
		},
		{
			name:          "missing source",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1}`,
			want:          400,
		},
		{
			name:          "invalid version",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":0,"source":"new"}`,
			want:          400,
		},
		{
			name:          "empty source",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":" "}`,
			want:          422,
		},
		{
			name:          "trailing YAML",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":"---\ntitle: first\n...\nowner: other\n---\nbody"}`,
			want:          422,
		},
		{
			name:          "stale version",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":2,"source":"new"}`,
			want:          409,
		},
		{
			name:          "multiple documents",
			method:        "PUT",
			path:          "/api/v1/knowledge/" + id,
			authenticated: true,
			origin:        "https://app.test",
			body:          `{"version":1,"source":"new"}{}`,
			want:          400,
		},
		{
			name:   "preview on app host",
			method: "GET",
			path:   "/private/html?ticket=invalid",
			want:   404,
		},
		{
			name:   "invalid preview",
			method: "GET",
			path:   "/private/html?ticket=invalid",
			host:   "preview.test",
			want:   404,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter(
				usecase.NewReadinessUseCase(fakeReadinessChecker{}),
				slog.New(slog.NewTextHandler(io.Discard, nil)),
			)

			var auth Authenticate
			if test.authenticated {
				auth = func(*gin.Context) (domain.Principal, error) {
					return domain.Principal{OwnerID: "owner", SessionID: "session"}, nil
				}
			}

			RegisterKnowledge(router, u, auth, "https://app.test", "https://preview.test")

			host := test.host
			if host == "" {
				host = "app.test"
			}

			request := httptest.NewRequest(test.method, "https://"+host+test.path, strings.NewReader(test.body))
			request.Header.Set("Origin", test.origin)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.want, recorder.Code, recorder.Body.String())
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			require.Empty(t, recorder.Header().Get("Set-Cookie"))
			require.Empty(t, recorder.Header().Get("ETag"))

			if test.host == "preview.test" {
				require.Contains(
					t,
					recorder.Header().Get("Content-Security-Policy"),
					"frame-ancestors https://app.test",
				)
				require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
			}

			if test.want == http.StatusOK {
				require.NotContains(t, recorder.Body.String(), "sourceKey")
				require.NotContains(t, recorder.Body.String(), "ownerId")
			}
		})
	}
}
