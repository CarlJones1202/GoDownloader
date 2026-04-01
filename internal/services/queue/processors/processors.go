// Package processors wires concrete queue.Processor implementations for each
// queue type (image, video, gallery, crawl) to the queue.Manager.
//
// Usage:
//
//	reg := ripper.NewRegistry(cfg.Storage.ImagesDir, client)
//	providers.RegisterAll(reg, client, cfg.Crawler.UserAgent)
//
//	procs := processors.New(db, reg, cfg)
//	procs.Register(queueMgr)
package processors

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/queue"
	"github.com/carlj/godownload/internal/services/ripper"
)

// Processors holds all queue processor implementations.
type Processors struct {
	db  *database.DB
	reg *ripper.Registry
	cfg config.Config
}

// New creates a Processors instance.
func New(db *database.DB, reg *ripper.Registry, cfg config.Config) *Processors {
	return &Processors{db: db, reg: reg, cfg: cfg}
}

// Register binds all processors to the queue Manager.
func (p *Processors) Register(mgr *queue.Manager) {
	mgr.RegisterProcessor(models.QueueTypeImage, queue.ProcessorFunc(p.processImage))
	mgr.RegisterProcessor(models.QueueTypeVideo, queue.ProcessorFunc(p.processVideo))
	mgr.RegisterProcessor(models.QueueTypeGallery, queue.ProcessorFunc(p.processGallery))
	mgr.RegisterProcessor(models.QueueTypeCrawl, queue.ProcessorFunc(p.processCrawl))
}

// processImage downloads a single image page URL, saves the file, and
// creates or updates an Image record in the database.
func (p *Processors) processImage(ctx context.Context, item *models.DownloadQueue) error {
	// The URL may contain a pipe-separated thumbnail URL appended by the
	// crawler: "pageURL|thumbnailURL". Split them so the ripper can use
	// the thumbnail for URL-transform providers (AcidImg, PixHost, etc.).
	pageURL, thumbnailURL := splitQueueURL(item.URL)

	slog.Info("processor: downloading image", "url", pageURL, "queue_id", item.ID)

	results, err := p.reg.DownloadWithThumbnail(ctx, pageURL, thumbnailURL)
	if err != nil {
		return fmt.Errorf("processor: image download %q: %w", pageURL, err)
	}

	if len(results) == 0 {
		return fmt.Errorf("processor: no files downloaded from %q", pageURL)
	}

	// Store only the page URL as the original URL in the DB.
	for _, res := range results {
		img := &models.Image{
			GalleryID:   item.TargetID,
			Filename:    res.Filename,
			OriginalURL: &pageURL,
			FileHash:    &res.FileHash,
			IsVideo:     isVideoContentType(res.ContentType),
			VRMode:      string(models.VRModeNone),
		}

		if err := p.db.CreateImage(ctx, img); err != nil {
			slog.Error("processor: saving image record", "error", err, "file", res.Filename)
			// Non-fatal: log and continue with remaining results.
			continue
		}

		slog.Info("processor: image saved",
			"image_id", img.ID,
			"filename", res.Filename,
			"hash", res.FileHash,
		)
	}

	return nil
}

// splitQueueURL splits a queue URL that may contain a pipe-separated
// thumbnail URL: "pageURL|thumbnailURL" -> (pageURL, thumbnailURL).
// If there is no pipe, thumbnailURL is empty.
func splitQueueURL(raw string) (pageURL, thumbnailURL string) {
	if idx := strings.Index(raw, "|"); idx >= 0 {
		return raw[:idx], raw[idx+1:]
	}
	return raw, ""
}

// processVideo downloads a video by delegating to the same ripper registry.
// Videos are stored in the configured videos directory.
func (p *Processors) processVideo(ctx context.Context, item *models.DownloadQueue) error {
	slog.Info("processor: downloading video", "url", item.URL, "queue_id", item.ID)

	// Override destination to videos directory by creating a separate registry
	// view — here we reuse the same registry but note the destination is
	// determined by the registry's configured destDir. For videos, callers
	// should enqueue items with the video ripper registry pointed at the videos dir.
	// For now, we delegate to the shared registry (Phase 3 will differentiate).
	results, err := p.reg.Download(ctx, item.URL)
	if err != nil {
		return fmt.Errorf("processor: video download %q: %w", item.URL, err)
	}

	if len(results) == 0 {
		return fmt.Errorf("processor: no video files downloaded from %q", item.URL)
	}

	for _, res := range results {
		img := &models.Image{
			GalleryID:   item.TargetID,
			Filename:    res.Filename,
			OriginalURL: &item.URL,
			FileHash:    &res.FileHash,
			IsVideo:     true,
			VRMode:      string(models.VRModeNone),
		}

		if err := p.db.CreateImage(ctx, img); err != nil {
			slog.Error("processor: saving video record", "error", err, "file", res.Filename)
			continue
		}

		slog.Info("processor: video saved",
			"image_id", img.ID,
			"filename", res.Filename,
		)
	}

	return nil
}

// processGallery handles a gallery-level download item. In Phase 2 this
// creates the gallery record if target_id is not yet set and enqueues
// individual image download tasks for each resolved direct image URL.
func (p *Processors) processGallery(ctx context.Context, item *models.DownloadQueue) error {
	slog.Info("processor: processing gallery", "url", item.URL, "queue_id", item.ID)

	// Resolve direct image URLs from the gallery page.
	results, err := p.reg.Download(ctx, item.URL)
	if err != nil {
		return fmt.Errorf("processor: gallery rip %q: %w", item.URL, err)
	}

	// Determine the gallery to attach images to.
	var galleryID *int64
	if item.TargetID != nil {
		galleryID = item.TargetID
	} else {
		// Create a stub gallery record so images have somewhere to live.
		title := filepath.Base(item.URL)
		gallery := &models.Gallery{
			URL:   &item.URL,
			Title: &title,
		}
		if err := p.db.CreateGallery(ctx, gallery); err != nil {
			return fmt.Errorf("processor: creating gallery record: %w", err)
		}
		galleryID = &gallery.ID
		slog.Info("processor: created stub gallery", "gallery_id", gallery.ID, "url", item.URL)
	}

	// Persist each downloaded file as an Image record.
	for _, res := range results {
		img := &models.Image{
			GalleryID:   galleryID,
			Filename:    res.Filename,
			OriginalURL: &item.URL,
			FileHash:    &res.FileHash,
			IsVideo:     isVideoContentType(res.ContentType),
			VRMode:      string(models.VRModeNone),
		}

		if err := p.db.CreateImage(ctx, img); err != nil {
			slog.Error("processor: saving gallery image", "error", err, "file", res.Filename)
			continue
		}
	}

	slog.Info("processor: gallery processed",
		"url", item.URL,
		"files_downloaded", len(results),
	)
	return nil
}

// processCrawl handles a crawl queue item. In Phase 2 this is still a stub
// that logs the crawl intent. Provider-specific scraping is wired in Phase 3.
func (p *Processors) processCrawl(ctx context.Context, item *models.DownloadQueue) error {
	slog.Info("processor: crawl stub — no-op until Phase 3",
		"url", item.URL,
		"source_id", item.TargetID,
		"queue_id", item.ID,
	)
	// Update last_crawled_at if we have a source ID.
	if item.TargetID != nil {
		if err := p.db.TouchSourceCrawledAt(ctx, *item.TargetID); err != nil {
			return fmt.Errorf("processor: touching source %d crawled_at: %w", *item.TargetID, err)
		}
	}
	return nil
}

// isVideoContentType returns true when the MIME type indicates a video file.
func isVideoContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "video/") ||
		strings.Contains(ct, "mp4") ||
		strings.Contains(ct, "webm") ||
		strings.Contains(ct, "avi") ||
		strings.Contains(ct, "mkv")
}
