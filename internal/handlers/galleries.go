// Package handlers — gallery HTTP handlers.
package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/gin-gonic/gin"
)

// GalleryHandler handles HTTP requests for the /api/v1/galleries resource.
type GalleryHandler struct {
	db      *database.DB
	storage config.StorageConfig
}

// NewGalleryHandler creates a GalleryHandler.
func NewGalleryHandler(db *database.DB, storage config.StorageConfig) *GalleryHandler {
	return &GalleryHandler{db: db, storage: storage}
}

// RegisterRoutes registers all gallery routes on the given group.
func (h *GalleryHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.GET("/:id", h.get)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
	rg.POST("/:id/images", h.addImage)
	rg.GET("/:id/people", h.listPeople)
}

type createGalleryRequest struct {
	SourceID          *int64  `json:"source_id"`
	Provider          *string `json:"provider"`
	ProviderGalleryID *string `json:"provider_gallery_id"`
	Title             *string `json:"title"`
	URL               *string `json:"url"`
	ThumbnailURL      *string `json:"thumbnail_url"`
}

type updateGalleryRequest struct {
	SourceID           *int64  `json:"source_id"`
	Provider           *string `json:"provider"`
	ProviderGalleryID  *string `json:"provider_gallery_id"`
	Title              *string `json:"title"`
	URL                *string `json:"url"`
	ThumbnailURL       *string `json:"thumbnail_url"`
	LocalThumbnailPath *string `json:"local_thumbnail_path"`
}

type addImageToGalleryRequest struct {
	Filename    string  `json:"filename"     binding:"required"`
	OriginalURL *string `json:"original_url"`
	IsVideo     bool    `json:"is_video"`
}

func (h *GalleryHandler) list(c *gin.Context) {
	limit, offset := paginationParams(c)

	f := database.GalleryFilter{
		Limit:  limit,
		Offset: offset,
	}
	if v := c.Query("source_id"); v != "" {
		if id, ok := strToInt64(v); ok {
			f.SourceID = &id
		}
	}
	if v := c.Query("provider"); v != "" {
		f.Provider = &v
	}
	if v := c.Query("search"); v != "" {
		f.Search = &v
	}

	galleries, err := h.db.ListGalleries(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, galleries)
}

func (h *GalleryHandler) get(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	g, err := h.db.GetGallery(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, g)
}

func (h *GalleryHandler) create(c *gin.Context) {
	var req createGalleryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	g := &models.Gallery{
		SourceID:          req.SourceID,
		Provider:          req.Provider,
		ProviderGalleryID: req.ProviderGalleryID,
		Title:             req.Title,
		URL:               req.URL,
		ThumbnailURL:      req.ThumbnailURL,
	}

	if err := h.db.CreateGallery(c.Request.Context(), g); err != nil {
		handleDBError(c, err)
		return
	}
	respondCreated(c, g)
}

func (h *GalleryHandler) update(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	g, err := h.db.GetGallery(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	var req updateGalleryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.SourceID != nil {
		g.SourceID = req.SourceID
	}
	if req.Provider != nil {
		g.Provider = req.Provider
	}
	if req.ProviderGalleryID != nil {
		g.ProviderGalleryID = req.ProviderGalleryID
	}
	if req.Title != nil {
		g.Title = req.Title
	}
	if req.URL != nil {
		g.URL = req.URL
	}
	if req.ThumbnailURL != nil {
		g.ThumbnailURL = req.ThumbnailURL
	}
	if req.LocalThumbnailPath != nil {
		g.LocalThumbnailPath = req.LocalThumbnailPath
	}

	if err := h.db.UpdateGallery(c.Request.Context(), g); err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, g)
}

func (h *GalleryHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	// List all images in the gallery so we can delete their files from disk
	// before the cascade delete removes the DB rows.
	galleryImages, err := h.db.ListImages(ctx, database.ImageFilter{GalleryID: &id, Limit: 100_000})
	if err != nil {
		slog.Warn("delete gallery: failed to list images for file cleanup", "gallery_id", id, "error", err)
		// Continue — we still want to delete the DB records even if file listing fails.
	}

	// Delete each image file from disk.
	for _, img := range galleryImages {
		dir := h.storage.ImagesDir
		if img.IsVideo {
			dir = h.storage.VideosDir
		}
		fp := filepath.Join(dir, img.Filename)
		if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
			slog.Warn("delete gallery: failed to remove file", "path", fp, "error", err)
		}
	}

	// Delete the gallery thumbnail if it exists.
	gallery, err := h.db.GetGallery(ctx, id)
	if err == nil && gallery.LocalThumbnailPath != nil && *gallery.LocalThumbnailPath != "" {
		tp := filepath.Join(h.storage.ThumbnailsDir, *gallery.LocalThumbnailPath)
		if err := os.Remove(tp); err != nil && !os.IsNotExist(err) {
			slog.Warn("delete gallery: failed to remove thumbnail", "path", tp, "error", err)
		}
	}

	// Delete the gallery row; SQLite ON DELETE CASCADE removes images, gallery_persons, etc.
	if err := h.db.DeleteGallery(ctx, id); err != nil {
		handleDBError(c, err)
		return
	}
	respondNoContent(c)
}

func (h *GalleryHandler) addImage(c *gin.Context) {
	galleryID, ok := parseIDParam(c)
	if !ok {
		return
	}

	if _, err := h.db.GetGallery(c.Request.Context(), galleryID); err != nil {
		handleDBError(c, err)
		return
	}

	var req addImageToGalleryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	img := &models.Image{
		GalleryID:   &galleryID,
		Filename:    req.Filename,
		OriginalURL: req.OriginalURL,
		IsVideo:     req.IsVideo,
		VRMode:      string(models.VRModeNone),
	}

	if err := h.db.CreateImage(c.Request.Context(), img); err != nil {
		handleDBError(c, err)
		return
	}
	respondCreated(c, img)
}

func (h *GalleryHandler) listPeople(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	people, err := h.db.ListGalleryPeople(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, people)
}

// strToInt64 parses a string as int64, returning false on failure.
func strToInt64(s string) (int64, bool) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int64(ch-'0')
	}
	return n, len(s) > 0
}
