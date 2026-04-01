// Package handlers — image HTTP handlers.
package handlers

import (
	"net/http"
	"strconv"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/gin-gonic/gin"
)

// ImageHandler handles HTTP requests for the /api/v1/images resource.
type ImageHandler struct {
	db *database.DB
}

// NewImageHandler creates an ImageHandler.
func NewImageHandler(db *database.DB) *ImageHandler {
	return &ImageHandler{db: db}
}

// RegisterRoutes registers all image routes on the given group.
func (h *ImageHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.GET("/search/color", h.searchByColor)
	rg.GET("/:id", h.get)
	rg.DELETE("/:id", h.delete)
	rg.POST("/:id/favorite", h.toggleFavorite)
	rg.POST("/:id/redownload", h.redownload)
}

func (h *ImageHandler) list(c *gin.Context) {
	limit, offset := paginationParams(c)

	f := database.ImageFilter{
		Limit:  limit,
		Offset: offset,
	}
	if v := c.Query("gallery_id"); v != "" {
		if id, ok := strToInt64(v); ok {
			f.GalleryID = &id
		}
	}
	if v := c.Query("is_video"); v != "" {
		b := v == "true" || v == "1"
		f.IsVideo = &b
	}
	if v := c.Query("is_favorite"); v != "" {
		b := v == "true" || v == "1"
		f.IsFavorite = &b
	}

	images, err := h.db.ListImages(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, images)
}

func (h *ImageHandler) get(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	img, err := h.db.GetImage(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, img)
}

func (h *ImageHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.db.DeleteImage(c.Request.Context(), id); err != nil {
		handleDBError(c, err)
		return
	}
	respondNoContent(c)
}

func (h *ImageHandler) toggleFavorite(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	isFavorite, err := h.db.ToggleFavorite(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{"id": id, "is_favorite": isFavorite})
}

// searchByColor finds images whose dominant palette contains colors similar to
// the provided hex color.
//
// Query params:
//   - color: hex color string, e.g. "ff0000" or "#ff0000" (required)
//   - max_distance: maximum squared RGB distance threshold (optional, default 0 = no limit)
//   - limit: max results to return (optional, default 50)
func (h *ImageHandler) searchByColor(c *gin.Context) {
	color := c.Query("color")
	if color == "" {
		respondError(c, http.StatusBadRequest, "color query parameter is required")
		return
	}

	maxDist := 0.0
	if v := c.Query("max_distance"); v != "" {
		if d, err := strconv.ParseFloat(v, 64); err == nil && d > 0 {
			maxDist = d
		}
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	results, err := h.db.SearchImagesByColor(c.Request.Context(), color, maxDist, limit)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	respondOK(c, results)
}

// redownload re-enqueues a download task for a single image by ID.
func (h *ImageHandler) redownload(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	img, err := h.db.GetImage(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	if img.OriginalURL == nil {
		respondError(c, http.StatusBadRequest, "image has no original URL to redownload")
		return
	}

	item := &models.DownloadQueue{
		Type:     string(models.QueueTypeImage),
		URL:      *img.OriginalURL,
		TargetID: img.GalleryID,
	}
	if err := h.db.EnqueueItem(c.Request.Context(), item); err != nil {
		handleDBError(c, err)
		return
	}

	respondOK(c, gin.H{
		"message":  "redownload enqueued",
		"image_id": id,
		"queue_id": item.ID,
	})
}
