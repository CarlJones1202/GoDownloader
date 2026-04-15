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
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/providers"
	"github.com/gin-gonic/gin"
)

// GalleryHandler handles HTTP requests for the /api/v1/galleries resource.
type GalleryHandler struct {
	db          *database.DB
	storage     config.StorageConfig
	metadataSvc *providers.GalleryMetadataService
	linker      *linker.AutoLinker
}

// NewGalleryHandler creates a GalleryHandler.
func NewGalleryHandler(db *database.DB, storage config.StorageConfig, metadataSvc *providers.GalleryMetadataService, al *linker.AutoLinker) *GalleryHandler {
	return &GalleryHandler{db: db, storage: storage, metadataSvc: metadataSvc, linker: al}
}

// RegisterRoutes registers all gallery routes on the given group.
func (h *GalleryHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.GET("/metadata-providers", h.metadataProviders)
	rg.GET("/:id", h.get)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
	rg.POST("/:id/images", h.addImage)
	rg.GET("/:id/people", h.listPeople)
	rg.GET("/:id/search-metadata", h.searchMetadata)
	rg.POST("/:id/scrape-metadata", h.scrapeMetadata)
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
	SourceID             *int64   `json:"source_id"`
	Provider             *string  `json:"provider"`
	ProviderGalleryID    *string  `json:"provider_gallery_id"`
	Title                *string  `json:"title"`
	URL                  *string  `json:"url"`
	ThumbnailURL         *string  `json:"thumbnail_url"`
	LocalThumbnailPath   *string  `json:"local_thumbnail_path"`
	Description          *string  `json:"description"`
	Rating               *float64 `json:"rating"`
	ReleaseDate          *string  `json:"release_date"`
	SourceURL            *string  `json:"source_url"`
	ProviderThumbnailURL *string  `json:"provider_thumbnail_url"`
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
	totalCount, err := h.db.CountGalleries(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	totalPages := (totalCount + int64(limit) - 1) / int64(limit) // ceil division
	currentPage := (offset / limit) + 1
	respondOK(c, gin.H{
		"items": galleries,
		"total_items": totalCount,
		"total_pages": totalPages,
		"current_page": currentPage,
		"page_size": limit,
	})
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

	// Auto-link new gallery to people
	if h.linker != nil {
		go h.linker.LinkGallery(c.Request.Context(), g) //nolint:errcheck
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
	if req.Description != nil {
		g.Description = req.Description
	}
	if req.Rating != nil {
		g.Rating = req.Rating
	}
	if req.ReleaseDate != nil {
		g.ReleaseDate = req.ReleaseDate
	}
	if req.SourceURL != nil {
		g.SourceURL = req.SourceURL
	}
	if req.ProviderThumbnailURL != nil {
		g.ProviderThumbnailURL = req.ProviderThumbnailURL
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

	// Delete each image file and its thumbnail from disk.
	for _, img := range galleryImages {
		dir := h.storage.ImagesDir
		if img.IsVideo {
			dir = h.storage.VideosDir
		}
		fp := filepath.Join(dir, img.Filename)
		if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
			slog.Warn("delete gallery: failed to remove file", "path", fp, "error", err)
		}

		// Remove corresponding thumbnail.
		if !img.IsVideo {
			thumbName := thumbnailFilename(img.Filename)
			tp := filepath.Join(h.storage.ThumbnailsDir, thumbName)
			if err := os.Remove(tp); err != nil && !os.IsNotExist(err) {
				slog.Warn("delete gallery: failed to remove thumbnail", "path", tp, "error", err)
			}
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

// searchMetadata searches all gallery metadata providers for matching galleries.
// GET /api/v1/galleries/:id/search-metadata?query=...
func (h *GalleryHandler) searchMetadata(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	// Verify gallery exists.
	gallery, err := h.db.GetGallery(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	query := c.Query("query")
	if query == "" {
		// Default to the gallery title if no query provided.
		if gallery.Title != nil && *gallery.Title != "" {
			query = *gallery.Title
		} else {
			respondError(c, http.StatusBadRequest, "query parameter is required when gallery has no title")
			return
		}
	}

	results, err := func() ([]providers.GallerySearchResult, error) {
		if provider := c.Query("provider"); provider != "" {
			return h.metadataSvc.SearchByProvider(c.Request.Context(), query, provider)
		}
		return h.metadataSvc.SearchAll(c.Request.Context(), query)
	}()
	if err != nil {
		slog.Debug("gallery metadata search returned no results", "gallery_id", id, "query", query, "error", err)
		// Return empty array instead of error — no results is not an error for the UI.
		respondOK(c, []providers.GallerySearchResult{})
		return
	}

	respondOK(c, results)
}

// scrapeMetadataRequest is the JSON body for the scrape-metadata endpoint.
type scrapeMetadataRequest struct {
	Provider string `json:"provider"  binding:"required"`
	URL      string `json:"url"       binding:"required"`
	SourceID string `json:"source_id"`
}

// scrapeMetadata scrapes full metadata from a specific provider URL and applies
// it to the gallery.
// POST /api/v1/galleries/:id/scrape-metadata
func (h *GalleryHandler) scrapeMetadata(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	// Verify gallery exists.
	gallery, err := h.db.GetGallery(ctx, id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	var req scrapeMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	meta, err := h.metadataSvc.ScrapeMetadata(ctx, req.URL, req.Provider, req.SourceID)
	if err != nil {
		slog.Warn("gallery metadata scrape failed", "gallery_id", id, "provider", req.Provider, "url", req.URL, "error", err)
		respondError(c, http.StatusBadGateway, "failed to scrape metadata: "+err.Error())
		return
	}

	// Apply scraped metadata to gallery.
	if meta.Description != "" {
		gallery.Description = &meta.Description
	}
	if meta.Rating > 0 {
		gallery.Rating = &meta.Rating
	}
	if !meta.ReleaseDate.IsZero() {
		d := meta.ReleaseDate.Format("2006-01-02")
		gallery.ReleaseDate = &d
	}
	if meta.SourceURL != "" {
		gallery.SourceURL = &meta.SourceURL
	}
	if meta.ThumbnailURL != "" {
		gallery.ProviderThumbnailURL = &meta.ThumbnailURL
	}

	if err := h.db.UpdateGallery(ctx, gallery); err != nil {
		handleDBError(c, err)
		return
	}

	slog.Info("gallery metadata applied", "gallery_id", id, "provider", req.Provider)
	respondOK(c, gallery)
}

// metadataProviders returns the list of supported metadata provider names.
// GET /api/v1/galleries/metadata-providers
func (h *GalleryHandler) metadataProviders(c *gin.Context) {
	respondOK(c, h.metadataSvc.ProviderNames())
}
