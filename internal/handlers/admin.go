// Package handlers — admin HTTP handlers (queue management, stats, cleanup).
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/crawler"
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/queue"
	"github.com/gin-gonic/gin"
)

// AdminHandler handles HTTP requests for the /api/v1/admin resource.
type AdminHandler struct {
	db              *database.DB
	crawler         *crawler.Crawler
	queueMgr        *queue.Manager
	linker          *linker.AutoLinker
	requestShutdown func(reason string) bool
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(db *database.DB, c *crawler.Crawler, qm *queue.Manager, al *linker.AutoLinker, requestShutdown func(reason string) bool) *AdminHandler {
	return &AdminHandler{db: db, crawler: c, queueMgr: qm, linker: al, requestShutdown: requestShutdown}
}

// RegisterRoutes registers all admin routes on the given group.
func (h *AdminHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stats", h.stats)
	rg.GET("/queue", h.listQueue)
	rg.GET("/queue/status", h.queueStatus)
	rg.GET("/queue/active", h.activeDownloads)
	rg.POST("/queue/pause", h.queuePause)
	rg.POST("/queue/resume", h.queueResume)
	rg.DELETE("/queue", h.clearQueue)
	rg.POST("/queue/:id/retry", h.retryQueueItem)
	rg.POST("/queue/retry-failed", h.queueRetryFailed)
	rg.DELETE("/queue/:id", h.deleteQueueItem)
	rg.POST("/sources/:id/recrawl", h.recrawlSource)
	rg.POST("/images/redownload", h.bulkRedownload)
	rg.POST("/galleries/cleanup", h.galleryCleanup)
	rg.POST("/galleries/autolink", h.autolinkGalleries)
	rg.POST("/server/stop", h.stopServer)
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

	galleryCount, err := h.db.CountGalleries(ctx, database.GalleryFilter{})
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

	// Build per-provider breakdown of active downloads.
	activeList := h.queueMgr.ActiveDownloads()
	providerCounts := make(map[string]int, len(activeList))
	for _, ad := range activeList {
		providerCounts[ad.Provider]++
	}

	respondOK(c, gin.H{
		"paused":          h.queueMgr.IsPaused(),
		"stats":           stats,
		"active_by_provider": providerCounts,
	})
}

// activeDownloads returns the list of queue items currently being processed.
func (h *AdminHandler) activeDownloads(c *gin.Context) {
	type activeDownloadResponse struct {
		ID        int64  `json:"id"`
		URL       string `json:"url"`
		Type      string `json:"type"`
		Provider  string `json:"provider"`
		StartedAt int64  `json:"started_at"` // unix millis
	}

	list := h.queueMgr.ActiveDownloads()
	out := make([]activeDownloadResponse, 0, len(list))
	for _, ad := range list {
		out = append(out, activeDownloadResponse{
			ID:        ad.ID,
			URL:       ad.URL,
			Type:      ad.Type,
			Provider:  ad.Provider,
			StartedAt: ad.StartedAt.UnixMilli(),
		})
	}
	respondOK(c, out)
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

func (h *AdminHandler) queueRetryFailed(c *gin.Context) {
	retried, err := h.db.RetryFailed(c.Request.Context())
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{"retried": retried})
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
	// Run in background to avoid HTTP timeout for large databases
	go func() {
		linked, err := h.linker.ScanAllGalleries(context.Background())
		if err != nil {
			slog.Error("autolink: background scan failed", "error", err)
		} else {
			slog.Info("autolink: background scan complete", "linked_count", linked)
		}
	}()

	respondOK(c, gin.H{
		"message": "autolink scan started in background",
	})
}

type stopServerRequest struct {
	Confirm string `json:"confirm"`
}

// stopServer requests a graceful server shutdown.
// Requires an explicit confirmation phrase in the request body.
func (h *AdminHandler) stopServer(c *gin.Context) {
	if h.requestShutdown == nil {
		respondError(c, http.StatusNotImplemented, "server stop is not configured")
		return
	}

	var req stopServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(strings.ToUpper(req.Confirm)) != "STOP" {
		respondError(c, http.StatusBadRequest, `confirmation must be "STOP"`)
		return
	}

	h.queueMgr.Pause()
	if !h.requestShutdown("requested via admin API") {
		respondError(c, http.StatusConflict, "shutdown already in progress")
		return
	}
	slog.Warn("admin: graceful server shutdown requested")
	respondOK(c, gin.H{"message": "server shutdown requested"})
}

func boolPtr(b bool) *bool { return &b }
