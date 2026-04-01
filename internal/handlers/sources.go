// Package handlers — source HTTP handlers.
package handlers

import (
	"net/http"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/crawler"
	"github.com/gin-gonic/gin"
)

// SourceHandler handles HTTP requests for the /api/v1/sources resource.
type SourceHandler struct {
	db      *database.DB
	crawler *crawler.Crawler
}

// NewSourceHandler creates a SourceHandler.
func NewSourceHandler(db *database.DB, c *crawler.Crawler) *SourceHandler {
	return &SourceHandler{db: db, crawler: c}
}

// RegisterRoutes registers all source routes on the given group.
func (h *SourceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.DELETE("/:id", h.delete)
	rg.POST("/:id/crawl", h.crawl)
	rg.POST("/:id/recrawl", h.recrawl)
}

// createSourceRequest is the JSON body for source creation.
type createSourceRequest struct {
	URL      string `json:"url"      binding:"required,url"`
	Name     string `json:"name"     binding:"required"`
	Enabled  *bool  `json:"enabled"`
	Priority int    `json:"priority"`
}

func (h *SourceHandler) list(c *gin.Context) {
	sources, err := h.db.ListSources(c.Request.Context())
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, sources)
}

func (h *SourceHandler) create(c *gin.Context) {
	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	s := &models.Source{
		URL:      req.URL,
		Name:     req.Name,
		Enabled:  enabled,
		Priority: req.Priority,
	}

	if err := h.db.CreateSource(c.Request.Context(), s); err != nil {
		handleDBError(c, err)
		return
	}
	respondCreated(c, s)
}

func (h *SourceHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.db.DeleteSource(c.Request.Context(), id); err != nil {
		handleDBError(c, err)
		return
	}
	respondNoContent(c)
}

func (h *SourceHandler) crawl(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	src, err := h.db.GetSource(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	h.crawler.EnqueueSource(src)
	respondOK(c, gin.H{"message": "crawl queued", "source_id": id})
}

func (h *SourceHandler) recrawl(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	src, err := h.db.GetSource(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	h.crawler.EnqueueSourceFull(src)
	respondOK(c, gin.H{"message": "full recrawl queued", "source_id": id})
}
