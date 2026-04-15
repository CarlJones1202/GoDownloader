// Package processors wires concrete queue.Processor implementations for each
// queue type (image, video, gallery, crawl) to the queue.Manager.
//
// Usage:
//
//	reg := ripper.NewRegistry(cfg.Storage.ImagesDir, client)
//	providers.RegisterAll(reg, client, cfg.Crawler.UserAgent)
//
//	procs := processors.New(db, dbWriter, reg, cfg)
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
	"github.com/carlj/godownload/internal/services/video"
	"github.com/carlj/godownload/internal/services/workers"
)

// DBWriter interface for database write operations.
type DBWriter interface {
	CreateImage(ctx context.Context, img *models.Image) error
	CreateGallery(ctx context.Context, g *models.Gallery) error
	SetGalleryThumbnail(ctx context.Context, galleryID int64, thumbPath string) error
	TouchSourceCrawledAt(ctx context.Context, id int64) error
	EnqueueItem(ctx context.Context, item *models.DownloadQueue) error
	FindImageByGalleryAndFilename(ctx context.Context, galleryID *int64, filename string) (*models.Image, error)
}

// Processors holds all queue processor implementations.
type Processors struct {
	db        *database.DB
	dbWriter  DBWriter
	reg       *ripper.Registry
	videoReg  *video.Registry
	cfg       config.Config
	thumb     *workers.ThumbnailWorker
	color     *workers.ColorWorker
	videoW    *workers.VideoWorker
	trickplay *workers.TrickplayWorker
}

// New creates a Processors instance.
func New(
	db *database.DB,
	dbWriter DBWriter,
	reg *ripper.Registry,
	cfg config.Config,
	thumb *workers.ThumbnailWorker,
	color *workers.ColorWorker,
	videoReg *video.Registry,
	videoW *workers.VideoWorker,
	trickplay *workers.TrickplayWorker,
) *Processors {
	return &Processors{
		db:        db,
		dbWriter:  dbWriter,
		reg:       reg,
		videoReg:  videoReg,
		cfg:       cfg,
		thumb:     thumb,
		color:     color,
		videoW:    videoW,
		trickplay: trickplay,
	}
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
		// Check if image already exists (e.g., from a previous download or re-download scenario).
		existingImg, err := p.dbWriter.FindImageByGalleryAndFilename(ctx, item.TargetID, res.Filename)
		if err == nil && existingImg != nil {
			slog.Info("processor: image already exists, skipping record creation",
				"image_id", existingImg.ID,
				"filename", res.Filename,
				"gallery_id", item.TargetID,
			)
			// Still run thumbnail/color extraction for the re-downloaded file.
			p.generateThumbnail(ctx, existingImg)
			p.extractColors(ctx, existingImg)
			continue
		}

		img := &models.Image{
			GalleryID:   item.TargetID,
			Filename:    res.Filename,
			OriginalURL: &pageURL,
			FileHash:    &res.FileHash,
			IsVideo:     isVideoContentType(res.ContentType),
			VRMode:      string(models.VRModeNone),
		}

		if err := p.dbWriter.CreateImage(ctx, img); err != nil {
			slog.Error("processor: saving image record", "error", err, "file", res.Filename)
			// Non-fatal: log and continue with remaining results.
			continue
		}

		slog.Info("processor: image saved",
			"image_id", img.ID,
			"filename", res.Filename,
			"hash", res.FileHash,
		)

		// Generate thumbnail and set as gallery cover (first-image-wins).
		p.generateThumbnail(ctx, img)

		// Extract dominant colors.
		p.extractColors(ctx, img)
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

// processVideo downloads a video using the dedicated video ripper registry,
// then runs post-processing: ffprobe metadata extraction, thumbnail generation,
// and trickplay sprite sheet creation.
func (p *Processors) processVideo(ctx context.Context, item *models.DownloadQueue) error {
	slog.Info("processor: downloading video", "url", item.URL, "queue_id", item.ID)

	// Use the dedicated video ripper registry if available.
	if p.videoReg == nil {
		return fmt.Errorf("processor: video registry not configured")
	}

	result, err := p.videoReg.Download(ctx, item.URL)
	if err != nil {
		return fmt.Errorf("processor: video download %q: %w", item.URL, err)
	}

	img := &models.Image{
		GalleryID:   item.TargetID,
		Filename:    result.Filename,
		OriginalURL: &item.URL,
		FileHash:    &result.FileHash,
		IsVideo:     true,
		VRMode:      string(models.VRModeNone),
	}

	if err := p.dbWriter.CreateImage(ctx, img); err != nil {
		return fmt.Errorf("processor: saving video record: %w", err)
	}

	slog.Info("processor: video saved",
		"image_id", img.ID,
		"filename", result.Filename,
		"hash", result.FileHash,
	)

	// Post-processing pipeline:
	// 1. Extract metadata (ffprobe) and generate thumbnail (ffmpeg).
	if p.videoW != nil {
		if err := p.videoW.ProcessVideo(ctx, img); err != nil {
			slog.Warn("processor: video post-processing failed",
				"image_id", img.ID, "error", err)
		}
	}

	// 2. Set as gallery thumbnail (first-video-wins, using the video thumbnail).
	if img.GalleryID != nil {
		thumbFilename := thumbnailName(img.Filename)
		if err := p.dbWriter.SetGalleryThumbnail(ctx, *img.GalleryID, thumbFilename); err != nil {
			slog.Warn("processor: setting gallery thumbnail for video",
				"gallery_id", *img.GalleryID, "error", err)
		}
	}

	// 3. Generate trickplay sprite sheet + WebVTT.
	if p.trickplay != nil {
		if err := p.trickplay.GenerateForVideo(ctx, img); err != nil {
			slog.Warn("processor: trickplay generation failed",
				"image_id", img.ID, "error", err)
		}
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
		if err := p.dbWriter.CreateGallery(ctx, gallery); err != nil {
			return fmt.Errorf("processor: creating gallery record: %w", err)
		}
		galleryID = &gallery.ID
		slog.Info("processor: created stub gallery", "gallery_id", gallery.ID, "url", item.URL)
	}

	// Persist each downloaded file as an Image record.
	for _, res := range results {
		// Check if image already exists (e.g., from a previous download or re-download scenario).
		existingImg, err := p.dbWriter.FindImageByGalleryAndFilename(ctx, galleryID, res.Filename)
		if err == nil && existingImg != nil {
			slog.Info("processor: gallery image already exists, skipping record creation",
				"image_id", existingImg.ID,
				"filename", res.Filename,
				"gallery_id", galleryID,
			)
			// Still run thumbnail/color extraction for the re-downloaded file.
			p.generateThumbnail(ctx, existingImg)
			p.extractColors(ctx, existingImg)
			continue
		}

		img := &models.Image{
			GalleryID:   galleryID,
			Filename:    res.Filename,
			OriginalURL: &item.URL,
			FileHash:    &res.FileHash,
			IsVideo:     isVideoContentType(res.ContentType),
			VRMode:      string(models.VRModeNone),
		}

		if err := p.dbWriter.CreateImage(ctx, img); err != nil {
			slog.Error("processor: saving gallery image", "error", err, "file", res.Filename)
			continue
		}

		// Generate thumbnail and set as gallery cover (first-image-wins).
		p.generateThumbnail(ctx, img)

		// Extract dominant colors.
		p.extractColors(ctx, img)
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
		if err := p.dbWriter.TouchSourceCrawledAt(ctx, *item.TargetID); err != nil {
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

// generateThumbnail runs the ThumbnailWorker for a freshly saved image and,
// on success, sets it as the gallery cover thumbnail (first-image-wins).
func (p *Processors) generateThumbnail(ctx context.Context, img *models.Image) {
	if p.thumb == nil {
		return
	}

	// Skip video files — thumbnail generation only applies to images.
	if img.IsVideo {
		return
	}

	if err := p.thumb.GenerateForImage(ctx, img); err != nil {
		slog.Warn("processor: thumbnail generation failed", "image_id", img.ID, "error", err)
		return
	}

	// Set as gallery thumbnail (first-image-wins via SetGalleryThumbnail).
	if img.GalleryID != nil {
		thumbFilename := thumbnailName(img.Filename)
		if err := p.dbWriter.SetGalleryThumbnail(ctx, *img.GalleryID, thumbFilename); err != nil {
			slog.Warn("processor: setting gallery thumbnail", "gallery_id", *img.GalleryID, "error", err)
		}
	}
}

// thumbnailName returns the thumbnail filename for a given image filename.
// Must match the convention in workers.ThumbnailWorker.
func thumbnailName(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return base + "_thumb.jpg"
}

// extractColors runs the ColorWorker for a freshly saved image.
func (p *Processors) extractColors(ctx context.Context, img *models.Image) {
	if p.color == nil {
		return
	}
	if err := p.color.ExtractForImage(ctx, img); err != nil {
		slog.Warn("processor: color extraction failed", "image_id", img.ID, "error", err)
	}
}
