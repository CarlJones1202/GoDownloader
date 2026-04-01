// Package handlers — image HTTP handlers.
package handlers

import (
	"net/http"

	"github.com/carlj/godownload/internal/database"
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

// searchByColor is a placeholder for future color-based image search.
// The dominant_colors field stores extracted colors; range queries
// will be implemented once the extraction worker is complete.
func (h *ImageHandler) searchByColor(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "color search not yet implemented")
}
