package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/interface/response"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

// Authenticate は#19で接続し、JWTを検証する。検証せずにリクエストの
// ヘッダーやクエリから所有者・セッションを取得してはならない。
type (
	Authenticate     func(*gin.Context) (domain.Principal, error)
	KnowledgeHandler struct {
		usecase       *usecase.KnowledgeUseCase
		authenticate  Authenticate
		appOrigin     string
		previewOrigin string
	}
)

func RegisterKnowledge(
	router *gin.Engine,
	u *usecase.KnowledgeUseCase,
	auth Authenticate,
	appOrigin, previewOrigin string,
) {
	h := &KnowledgeHandler{usecase: u, authenticate: auth, appOrigin: appOrigin, previewOrigin: previewOrigin}
	api := router.Group("/api/v1", h.appHost, h.authorize)
	api.GET("/knowledge/recent", h.recent)
	api.GET("/knowledge", h.list)
	api.POST("/knowledge-drafts", h.upload)
	api.GET("/knowledge/:id", h.get)
	api.PUT("/knowledge/:id", h.save)
	api.GET("/knowledge-drafts/:id", h.draft)
	api.POST("/knowledge-drafts/:id/commit", h.commit)
	api.PUT("/knowledge/:id/visibility", h.visibility)
	api.POST("/knowledge/:id/preview-ticket", h.ticket)
	api.PUT("/knowledge/:id/folder", h.move)
	api.GET("/folders", h.folders)
	api.POST("/folders", h.createFolder)
	api.PATCH("/folders/:id", h.updateFolder)
	api.DELETE("/folders/:id", h.deleteFolder)

	router.GET("/private/html", h.previewHost, h.privateHTML)
	router.GET("/public/:publicId/html", h.previewHost, h.publicHTML)
}

func (h *KnowledgeHandler) appHost(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	if h.appOrigin != "" {
		parsed, _ := url.Parse(h.appOrigin)
		if parsed == nil || c.Request.Host != parsed.Host {
			response.WriteError(c, domain.ErrNotFound)
			return
		}
	}

	c.Next()
}

func (h *KnowledgeHandler) authorize(c *gin.Context) {
	if h.authenticate == nil {
		response.WriteError(c, domain.ErrUnauthenticated)
		return
	}

	p, err := h.authenticate(c)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	if c.Request.Method != "GET" && (h.appOrigin == "" || c.GetHeader("Origin") != h.appOrigin) {
		response.WriteError(c, domain.ErrForbidden)
		return
	}

	c.Set("knowledge.principal", p)
	c.Next()
}

func principal(c *gin.Context) domain.Principal {
	p, _ := c.Get("knowledge.principal")
	value, _ := p.(domain.Principal)

	return value
}

func resourceID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !domain.ValidID(id) {
		response.WriteError(c, domain.ErrBadRequest)
		return "", false
	}

	return id, true
}

func decode(c *gin.Context, v any) bool {
	// JSONのエスケープにより本文の1バイトが通信上6バイトになる場合がある。受信量を制限し、
	// 保存前にドメイン側で実際のUTF-8本文のサイズ制限を適用する。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 6*domain.MaxSourceBytes+4096)

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

	if err := decoder.Decode(v); err != nil {
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

func validVersion(c *gin.Context, v int64) bool {
	if v < 1 || v > domain.MaxVersion {
		response.WriteError(c, domain.ErrBadRequest)
		return false
	}

	return true
}

func (h *KnowledgeHandler) get(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	k, source, err := h.usecase.Get(c.Request.Context(), principal(c), id)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Detail(k, source, h.appOrigin, false))
}

func (h *KnowledgeHandler) recent(c *gin.Context) {
	items, err := h.usecase.Recent(c.Request.Context(), principal(c))
	if err != nil {
		response.WriteError(c, err)
		return
	}

	summaries := []response.RecentSummary{}

	for _, k := range items {
		summaries = append(
			summaries,
			response.RecentSummary{
				ID:        k.ID,
				Title:     k.Title,
				Format:    k.Format,
				UpdatedAt: k.UpdatedAt,
			},
		)
	}

	c.JSON(http.StatusOK, gin.H{"items": summaries})
}

func (h *KnowledgeHandler) list(c *gin.Context) {
	query := c.Query("query")

	page, pageSize := 1, 0

	var err error
	if value, ok := c.GetQuery("page"); ok {
		page, err = strconv.Atoi(value)
		if err != nil {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}
	}

	if value, ok := c.GetQuery("pageSize"); ok {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		if pageSize == 0 {
			pageSize = -1
		}
	}

	var folderID *string

	if value, ok := c.GetQuery("folderId"); ok {
		if value != "" && value != "root" && !domain.ValidID(value) {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		if value == "root" {
			value = ""
		}

		folderID = &value
	}

	pageResult, err := h.usecase.List(c.Request.Context(), principal(c), domain.ListQuery{
		Query: query, Tags: c.QueryArray("tag"), FolderID: folderID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		response.WriteError(c, err)
		return
	}

	items := make([]response.KnowledgeSummary, 0, len(pageResult.Items))
	for _, k := range pageResult.Items {
		tags := k.Tags
		if tags == nil {
			tags = []string{}
		}

		items = append(
			items,
			response.KnowledgeSummary{
				ID:        k.ID,
				Title:     k.Title,
				Format:    k.Format,
				Tags:      tags,
				Folder:    k.Folder,
				Version:   k.Version,
				UpdatedAt: k.UpdatedAt,
			},
		)
	}

	hasNext := pageResult.Total > (pageResult.Page-1)*pageResult.PageSize+len(items)

	c.JSON(
		http.StatusOK,
		response.KnowledgeList{
			Items:    items,
			Total:    pageResult.Total,
			Page:     pageResult.Page,
			PageSize: pageResult.PageSize,
			HasNext:  hasNext,
		},
	)
}

func (h *KnowledgeHandler) upload(c *gin.Context) {
	const multipartOverhead = 1 * 1024 * 1024

	maxMultipartBytes := int64(domain.MaxSourceBytes + multipartOverhead)

	if c.Request.ContentLength > maxMultipartBytes {
		response.WriteError(c, domain.ErrTooLarge)
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMultipartBytes)
	if err := c.Request.ParseMultipartForm(domain.MaxSourceBytes + multipartOverhead); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			response.WriteError(c, domain.ErrTooLarge)
		} else {
			response.WriteError(c, domain.ErrBadRequest)
		}

		return
	}

	var fileHeader *multipart.FileHeader

	count := 0
	for field, files := range c.Request.MultipartForm.File {
		count += len(files)
		if field == "file" && len(files) == 1 {
			fileHeader = files[0]
		}
	}

	if count != 1 || fileHeader == nil {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	if !validUploadContentType(fileHeader.Filename, fileHeader.Header.Get("Content-Type")) {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	data, readErr := io.ReadAll(io.LimitReader(file, domain.MaxSourceBytes+1))
	_ = file.Close()

	if readErr != nil {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	if len(data) > domain.MaxSourceBytes {
		response.WriteError(c, domain.ErrTooLarge)
		return
	}

	var folderID *string

	if value := strings.TrimSpace(c.PostForm("folderId")); value != "" {
		if !domain.ValidID(value) {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		folderID = &value
	}

	draft, err := h.usecase.Upload(c.Request.Context(), principal(c), fileHeader.Filename, string(data), folderID)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response.CreatedDraft(draft.DraftID))
}

func validUploadContentType(filename, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}

	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}

	mediaType = strings.ToLower(mediaType)
	if mediaType == "application/octet-stream" {
		return true
	}

	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md":
		return mediaType == "text/markdown" || mediaType == "text/plain"
	case ".html":
		return mediaType == "text/html" || mediaType == "application/xhtml+xml"
	default:
		return false
	}
}

func parseOptionalID(raw json.RawMessage) (*string, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}

	if string(bytes.TrimSpace(raw)) == "null" {
		return nil, true, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil || !domain.ValidID(value) {
		return nil, false, domain.ErrBadRequest
	}

	return &value, true, nil
}

func (h *KnowledgeHandler) folders(c *gin.Context) {
	items, err := h.usecase.Folders(c.Request.Context(), principal(c))
	if err != nil {
		response.WriteError(c, err)
		return
	}

	if items == nil {
		items = []domain.Folder{}
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

type folderRequest struct {
	Name     string          `json:"name"`
	ParentID json.RawMessage `json:"parentId"`
	Version  int64           `json:"version"`
}

func (h *KnowledgeHandler) createFolder(c *gin.Context) {
	var body folderRequest
	if !decode(c, &body) {
		return
	}

	parentID, _, err := parseOptionalID(body.ParentID)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	folder, err := h.usecase.CreateFolder(c.Request.Context(), principal(c), body.Name, parentID)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, folder)
}

func (h *KnowledgeHandler) updateFolder(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var body folderRequest
	if !decode(c, &body) || !validVersion(c, body.Version) {
		return
	}

	parentID, parentSpecified, err := parseOptionalID(body.ParentID)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	folder, err := h.usecase.UpdateFolder(
		c.Request.Context(),
		principal(c),
		id,
		body.Name,
		parentID,
		parentSpecified,
		body.Version,
	)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, folder)
}

func (h *KnowledgeHandler) deleteFolder(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var version int64

	if rawVersion, hasVersion := c.GetQuery("version"); hasVersion {
		parsed, err := strconv.ParseInt(rawVersion, 10, 64)
		if err != nil {
			response.WriteError(c, domain.ErrBadRequest)
			return
		}

		version = parsed
	} else {
		var body struct {
			Version int64 `json:"version"`
		}
		if !decode(c, &body) {
			return
		}

		version = body.Version
	}

	if !validVersion(c, version) {
		return
	}

	if err := h.usecase.DeleteFolder(c.Request.Context(), principal(c), id, version); err != nil {
		response.WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *KnowledgeHandler) move(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var body struct {
		FolderID json.RawMessage `json:"folderId"`
		Version  int64           `json:"version"`
	}
	if !decode(c, &body) || !validVersion(c, body.Version) {
		return
	}

	folderID, specified, err := parseOptionalID(body.FolderID)
	if err != nil || !specified {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	k, err := h.usecase.MoveKnowledge(c.Request.Context(), principal(c), id, folderID, body.Version)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Detail(k, "", h.appOrigin, false))
}

type sourceRequest struct {
	Version int64   `json:"version"`
	Source  *string `json:"source"`
}

func (h *KnowledgeHandler) save(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var body sourceRequest
	if !decode(c, &body) || !validVersion(c, body.Version) {
		return
	}

	if body.Source == nil {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	k, warning, err := h.usecase.Save(c.Request.Context(), principal(c), id, body.Version, *body.Source)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Detail(k, *body.Source, h.appOrigin, warning))
}

func (h *KnowledgeHandler) draft(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	d, source, err := h.usecase.Draft(c.Request.Context(), principal(c), id)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Draft(d, source))
}

func (h *KnowledgeHandler) commit(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var body sourceRequest
	if !decode(c, &body) || !validVersion(c, body.Version) {
		return
	}

	if body.Source == nil {
		response.WriteError(c, domain.ErrBadRequest)
		return
	}

	knowledgeID, created, err := h.usecase.Commit(c.Request.Context(), principal(c), id, body.Version, *body.Source)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	c.JSON(status, response.Committed(knowledgeID))
}

func (h *KnowledgeHandler) visibility(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var body struct {
		Version    int64  `json:"version"`
		Visibility string `json:"visibility"`
	}
	if !decode(c, &body) || !validVersion(c, body.Version) {
		return
	}

	k, source, err := h.usecase.Visibility(c.Request.Context(), principal(c), id, body.Version, body.Visibility)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.Detail(k, source, h.appOrigin, false))
}

func (h *KnowledgeHandler) ticket(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}

	var body struct {
		Version int64 `json:"version"`
	}
	if !decode(c, &body) || !validVersion(c, body.Version) {
		return
	}

	if h.previewOrigin == "" {
		response.WriteError(c, domain.ErrUnavailable)
		return
	}

	token, err := h.usecase.PreviewTicket(c.Request.Context(), principal(c), id, body.Version)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": h.previewOrigin + "/private/html?ticket=" + url.QueryEscape(token)})
}

func (h *KnowledgeHandler) previewHost(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")

	origin, err := url.Parse(h.previewOrigin)
	if err != nil || h.previewOrigin == "" || origin.Host != c.Request.Host {
		response.WriteError(c, domain.ErrNotFound)
		return
	}

	c.Header(
		"Content-Security-Policy",
		"default-src 'none'; script-src 'none'; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; style-src 'unsafe-inline'; img-src 'none'; font-src 'none'; frame-ancestors "+h.appOrigin,
	)
	c.Next()
}

func (h *KnowledgeHandler) privateHTML(c *gin.Context) {
	content, err := h.usecase.PrivateHTML(c.Request.Context(), c.Query("ticket"))
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func (h *KnowledgeHandler) publicHTML(c *gin.Context) {
	version, err := strconv.ParseInt(c.Query("version"), 10, 64)
	if err != nil || version < 1 || version > domain.MaxVersion || len(c.Param("publicId")) != 43 {
		response.WriteError(c, domain.ErrNotFound)
		return
	}

	content, err := h.usecase.PublicHTML(c.Request.Context(), c.Param("publicId"), version)
	if err != nil {
		response.WriteError(c, err)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}
