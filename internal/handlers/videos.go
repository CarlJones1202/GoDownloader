// Package handlers — video HTTP handlers.
// Videos are stored in the images table with is_video = true.
// These endpoints provide a video-specific view.
package handlers

import (
	"net/http"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/gin-gonic/gin"
)

// VideoHandler handles HTTP requests for the /api/v1/videos resource.
type VideoHandler struct {
	db *database.DB
}

// NewVideoHandler creates a VideoHandler.
func NewVideoHandler(db *database.DB) *VideoHandler {
	return &VideoHandler{db: db}
}

// RegisterRoutes registers all video routes on the given group.
func (h *VideoHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.GET("/:id", h.get)
	rg.POST("/:id/redownload", h.redownload)
}

// list returns paginated videos (images with is_video = true).
func (h *VideoHandler) list(c *gin.Context) {
	limit, offset := paginationParams(c)
	isVideo := true

	f := database.ImageFilter{
		Limit:   limit,
		Offset:  offset,
		IsVideo: &isVideo,
	}
	if v := c.Query("gallery_id"); v != "" {
		if id, ok := strToInt64(v); ok {
			f.GalleryID = &id
		}
	}
	if v := c.Query("is_favorite"); v != "" {
		b := v == "true" || v == "1"
		f.IsFavorite = &b
	}

	videos, err := h.db.ListImages(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, videos)
}

// get returns a single video by ID.
func (h *VideoHandler) get(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	img, err := h.db.GetImage(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	if !img.IsVideo {
		respondError(c, http.StatusNotFound, "not a video")
		return
	}

	respondOK(c, img)
}

// redownload enqueues a re-download for the given video by adding it back
// to the download queue.
func (h *VideoHandler) redownload(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	img, err := h.db.GetImage(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	if !img.IsVideo {
		respondError(c, http.StatusBadRequest, "not a video")
		return
	}

	if img.OriginalURL == nil || *img.OriginalURL == "" {
		respondError(c, http.StatusBadRequest, "video has no original URL to re-download")
		return
	}

	item := &models.DownloadQueue{
		Type:     string(models.QueueTypeVideo),
		URL:      *img.OriginalURL,
		TargetID: &img.ID,
	}

	if err := h.db.EnqueueItem(c.Request.Context(), item); err != nil {
		handleDBError(c, err)
		return
	}

	respondOK(c, gin.H{"message": "re-download queued", "queue_id": item.ID, "video_id": id})
}
