// Package handlers — image HTTP handlers.
package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/gin-gonic/gin"
)

// ImageHandler handles HTTP requests for the /api/v1/images resource.
type ImageHandler struct {
	db      *database.DB
	storage config.StorageConfig
}

// NewImageHandler creates an ImageHandler.
func NewImageHandler(db *database.DB, storage config.StorageConfig) *ImageHandler {
	return &ImageHandler{db: db, storage: storage}
}

func (h *ImageHandler) fileExists(img models.Image) bool {
	dir := h.storage.ImagesDir
	if img.IsVideo {
		dir = h.storage.VideosDir
	}
	_, err := os.Stat(filepath.Join(dir, img.Filename))
	return err == nil
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
		SortBy: database.SortByNewest,
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
	if v := c.Query("sort_by"); v != "" {
		f.SortBy = v
	}
	if v := c.Query("random_seed"); v != "" {
		if seed, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.RandomSeed = seed
		}
	}
	if v := c.Query("on_disk"); v != "" {
		f.OnDisk = v == "true" || v == "1"
	}

	var images []models.Image
	if f.OnDisk {
		var result []models.Image
		foundCount := 0
		skipCount := 0
		searchOffset := 0
		batchSize := 100
		if limit > batchSize {
			batchSize = limit * 2
		}

		ctx := c.Request.Context()
		for foundCount < limit {
			batch, err := h.db.ListImages(ctx, database.ImageFilter{
				GalleryID:  f.GalleryID,
				IsVideo:    f.IsVideo,
				IsFavorite: f.IsFavorite,
				SortBy:     f.SortBy,
				RandomSeed: f.RandomSeed,
				Limit:      batchSize,
				Offset:     searchOffset,
			})
			if err != nil || len(batch) == 0 {
				break
			}

			for _, img := range batch {
				if h.fileExists(img) {
					if skipCount < offset {
						skipCount++
					} else {
						result = append(result, img)
						foundCount++
						if foundCount == limit {
							break
						}
					}
				}
			}
			searchOffset += batchSize
			// Safety break to avoid infinite loop if no images exist
			if searchOffset > 100000 {
				break
			}
		}
		images = result
	} else {
		var err error
		images, err = h.db.ListImages(c.Request.Context(), f)
		if err != nil {
			handleDBError(c, err)
			return
		}
	}
	totalCount, err := h.db.CountImages(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	totalPages := (totalCount + int64(limit) - 1) / int64(limit)
	currentPage := (offset / limit) + 1
	respondOK(c, gin.H{
		"items": images,
		"total_items": totalCount,
		"total_pages": totalPages,
		"current_page": currentPage,
		"page_size": limit,
	})
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

	ctx := c.Request.Context()

	// Fetch the image first so we can delete the file from disk.
	img, err := h.db.GetImage(ctx, id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	// Determine the storage directory based on whether it's a video or image.
	dir := h.storage.ImagesDir
	if img.IsVideo {
		dir = h.storage.VideosDir
	}

	// Delete the file from disk. Log but don't fail if the file is already gone.
	filePath := filepath.Join(dir, img.Filename)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		slog.Warn("delete: failed to remove file", "path", filePath, "error", err)
	}

	// Also delete the thumbnail if it exists.
	thumbName := thumbnailFilename(img.Filename)
	thumbPath := filepath.Join(h.storage.ThumbnailsDir, thumbName)
	if err := os.Remove(thumbPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("delete: failed to remove thumbnail", "path", thumbPath, "error", err)
	}

	if err := h.db.DeleteImage(ctx, id); err != nil {
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

// thumbnailFilename returns the thumbnail filename for a given image filename.
// e.g. "photo.jpg" -> "photo_thumb.jpg". Must match the convention used by
// workers.ThumbnailWorker.
func thumbnailFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return base + "_thumb.jpg"
}
