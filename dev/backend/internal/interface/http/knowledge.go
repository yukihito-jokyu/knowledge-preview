package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	api.GET("/knowledge/:id", h.get)
	api.PUT("/knowledge/:id", h.save)
	api.GET("/knowledge-drafts/:id", h.draft)
	api.POST("/knowledge-drafts/:id/commit", h.commit)
	api.PUT("/knowledge/:id/visibility", h.visibility)
	api.POST("/knowledge/:id/preview-ticket", h.ticket)
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

	summaries := []response.KnowledgeSummary{}
	for _, k := range items {
		summaries = append(
			summaries,
			response.KnowledgeSummary{ID: k.ID, Title: k.Title, Format: k.Format, UpdatedAt: k.UpdatedAt},
		)
	}

	c.JSON(http.StatusOK, gin.H{"items": summaries})
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
