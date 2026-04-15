// Package handlers — admin HTTP handlers (queue management, stats, cleanup).
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/crawler"
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/queue"
	"github.com/gin-gonic/gin"
)

// AdminHandler handles HTTP requests for the /api/v1/admin resource.
type AdminHandler struct {
	db       *database.DB
	crawler  *crawler.Crawler
	queueMgr *queue.Manager
	linker   *linker.AutoLinker
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(db *database.DB, c *crawler.Crawler, qm *queue.Manager, al *linker.AutoLinker) *AdminHandler {
	return &AdminHandler{db: db, crawler: c, queueMgr: qm, linker: al}
}

// RegisterRoutes registers all admin routes on the given group.
func (h *AdminHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stats", h.stats)
	rg.GET("/queue", h.listQueue)
	rg.GET("/queue/status", h.queueStatus)
	rg.POST("/queue/pause", h.queuePause)
	rg.POST("/queue/resume", h.queueResume)
	rg.DELETE("/queue", h.clearQueue)
	rg.POST("/queue/:id/retry", h.retryQueueItem)
	rg.DELETE("/queue/:id", h.deleteQueueItem)
	rg.POST("/sources/:id/recrawl", h.recrawlSource)
	rg.POST("/images/redownload", h.bulkRedownload)
	rg.POST("/galleries/cleanup", h.galleryCleanup)
	rg.POST("/galleries/autolink", h.autolinkGalleries)
}

// stats returns aggregate statistics about the system.
// Response: gallery count, image count, video count, source count, people count,
// queue stats, download stats, and gallery provider breakdown.
func (h *AdminHandler) stats(c *gin.Context) {
	ctx := c.Request.Context()

	queueStats, err := h.db.GetQueueStats(ctx)
	if err != nil {
		handleDBError(c, err)
		return
	}

	downloadStats, err := h.db.GetDownloadStats(ctx)
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
		"downloads":          downloadStats,
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

// queueStatus returns the current queue status including paused state and counts.
func (h *AdminHandler) queueStatus(c *gin.Context) {
	stats, err := h.db.GetQueueStats(c.Request.Context())
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{
		"paused": h.queueMgr.IsPaused(),
		"stats":  stats,
	})
}

// queuePause pauses queue processing.
func (h *AdminHandler) queuePause(c *gin.Context) {
	h.queueMgr.Pause()
	slog.Info("admin: queue paused")
	respondOK(c, gin.H{"message": "queue paused", "paused": true})
}

// queueResume resumes queue processing.
func (h *AdminHandler) queueResume(c *gin.Context) {
	h.queueMgr.Resume()
	slog.Info("admin: queue resumed")
	respondOK(c, gin.H{"message": "queue resumed", "paused": false})
}

// clearQueue deletes queue items, optionally filtered by status.
//
// Query params:
//   - status: only delete items with this status (pending, completed, failed)
func (h *AdminHandler) clearQueue(c *gin.Context) {
	var statusFilter *string
	if v := c.Query("status"); v != "" {
		statusFilter = &v
	}

	deleted, err := h.db.ClearQueue(c.Request.Context(), statusFilter)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{"deleted": deleted})
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

// recrawlSource triggers a full re-crawl of a source by ID.
func (h *AdminHandler) recrawlSource(c *gin.Context) {
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
	slog.Info("admin: source recrawl enqueued", "source_id", id)
	respondOK(c, gin.H{"message": "recrawl enqueued", "source_id": id})
}

// bulkRedownload request body.
type bulkRedownloadRequest struct {
	ImageIDs []int64 `json:"image_ids" binding:"required"`
}

// bulkRedownload re-enqueues download tasks for multiple images.
func (h *AdminHandler) bulkRedownload(c *gin.Context) {
	var req bulkRedownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()
	enqueued := 0

	for _, imgID := range req.ImageIDs {
		img, err := h.db.GetImage(ctx, imgID)
		if err != nil {
			slog.Warn("admin: bulk redownload skip", "image_id", imgID, "error", err)
			continue
		}
		if img.OriginalURL == nil {
			continue
		}

		item := &models.DownloadQueue{
			Type:     string(models.QueueTypeImage),
			URL:      *img.OriginalURL,
			TargetID: img.GalleryID,
		}
		if err := h.db.EnqueueItem(ctx, item); err != nil {
			slog.Error("admin: bulk redownload enqueue", "image_id", imgID, "error", err)
			continue
		}
		enqueued++
	}

	respondOK(c, gin.H{
		"message":   "bulk redownload enqueued",
		"requested": len(req.ImageIDs),
		"enqueued":  enqueued,
	})
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

// autolinkGalleries triggers a global scan to link galleries to people based on
// name matches in titles and source URLs.
func (h *AdminHandler) autolinkGalleries(c *gin.Context) {
	linked, err := h.linker.ScanAllGalleries(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "autolink failed: "+err.Error())
		return
	}

	respondOK(c, gin.H{
		"message": "autolink scan complete",
		"linked":  linked,
	})
}

func boolPtr(b bool) *bool { return &b }
