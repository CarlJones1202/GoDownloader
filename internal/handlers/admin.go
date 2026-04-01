// Package handlers — admin HTTP handlers (queue management, stats, cleanup).
package handlers

import (
	"net/http"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/gin-gonic/gin"
)

// AdminHandler handles HTTP requests for the /api/v1/admin resource.
type AdminHandler struct {
	db *database.DB
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(db *database.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

// RegisterRoutes registers all admin routes on the given group.
func (h *AdminHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stats", h.stats)
	rg.GET("/queue", h.listQueue)
	rg.POST("/queue/:id/retry", h.retryQueueItem)
	rg.DELETE("/queue/:id", h.deleteQueueItem)
	rg.POST("/galleries/cleanup", h.galleryCleanup)
}

// stats returns aggregate statistics about the system.
// Response: gallery count, image count, video count, source count, people count,
// queue stats, and gallery provider breakdown.
func (h *AdminHandler) stats(c *gin.Context) {
	ctx := c.Request.Context()

	queueStats, err := h.db.GetQueueStats(ctx)
	if err != nil {
		handleDBError(c, err)
		return
	}

	imageCount, err := h.db.CountImages(ctx, database.ImageFilter{})
	if err != nil {
		handleDBError(c, err)
		return
	}

	videoCount, err := h.db.CountImages(ctx, database.ImageFilter{IsVideo: boolPtr(true)})
	if err != nil {
		handleDBError(c, err)
		return
	}

	galleryCount, err := h.db.CountGalleries(ctx)
	if err != nil {
		handleDBError(c, err)
		return
	}

	sourceCount, err := h.db.CountSources(ctx)
	if err != nil {
		handleDBError(c, err)
		return
	}

	peopleCount, err := h.db.CountPeople(ctx)
	if err != nil {
		handleDBError(c, err)
		return
	}

	providerBreakdown, err := h.db.GalleryProviderBreakdown(ctx)
	if err != nil {
		handleDBError(c, err)
		return
	}

	favoriteCount, err := h.db.CountImages(ctx, database.ImageFilter{IsFavorite: boolPtr(true)})
	if err != nil {
		handleDBError(c, err)
		return
	}

	respondOK(c, gin.H{
		"sources":            sourceCount,
		"galleries":          galleryCount,
		"images":             imageCount,
		"videos":             videoCount,
		"people":             peopleCount,
		"favorites":          favoriteCount,
		"queue":              queueStats,
		"provider_breakdown": providerBreakdown,
	})
}

func (h *AdminHandler) listQueue(c *gin.Context) {
	limit, offset := paginationParams(c)

	f := database.QueueFilter{
		Limit:  limit,
		Offset: offset,
	}
	if v := c.Query("status"); v != "" {
		f.Status = &v
	}
	if v := c.Query("type"); v != "" {
		f.Type = &v
	}

	items, err := h.db.ListQueue(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, items)
}

func (h *AdminHandler) retryQueueItem(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	item, err := h.db.GetQueueItem(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	if models.QueueStatus(item.Status) != models.QueueStatusFailed {
		respondError(c, http.StatusBadRequest, "only failed items can be retried")
		return
	}

	if err := h.db.IncrementRetry(c.Request.Context(), id); err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{"message": "queued for retry", "id": id})
}

func (h *AdminHandler) deleteQueueItem(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.db.DeleteQueueItem(c.Request.Context(), id); err != nil {
		handleDBError(c, err)
		return
	}
	respondNoContent(c)
}

// galleryCleanup finds and optionally removes orphaned images that are not
// linked to any gallery.
//
// Query params:
//   - dry_run: if "true" (default), only lists orphans without deleting
func (h *AdminHandler) galleryCleanup(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	if dryRun {
		orphans, err := h.db.ListOrphanedImages(c.Request.Context())
		if err != nil {
			handleDBError(c, err)
			return
		}
		respondOK(c, gin.H{
			"dry_run": true,
			"count":   len(orphans),
			"orphans": orphans,
		})
		return
	}

	deleted, err := h.db.DeleteOrphanedImages(c.Request.Context())
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{
		"dry_run": false,
		"deleted": deleted,
	})
}

func boolPtr(b bool) *bool { return &b }
