// Package main is the entrypoint for the GoDownload API server.
// It loads configuration, initialises dependencies, registers routes,
// and starts the HTTP server with graceful shutdown.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/handlers"
	"github.com/carlj/godownload/internal/middleware"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/crawler"
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/personphoto"
	"github.com/carlj/godownload/internal/services/providers"
	"github.com/carlj/godownload/internal/services/queue"
	"github.com/carlj/godownload/internal/services/queue/processors"
	"github.com/carlj/godownload/internal/services/ripper"
	ripperproviders "github.com/carlj/godownload/internal/services/ripper/providers"
	"github.com/carlj/godownload/internal/services/video"
	"github.com/carlj/godownload/internal/services/vpn"
	"github.com/carlj/godownload/internal/services/workers"
	"github.com/carlj/godownload/internal/services/ws"
	"github.com/carlj/godownload/internal/utils"
	"github.com/gin-gonic/gin"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.Log)

	db, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	autoLinker := linker.New(db)

	crawlerSvc := crawler.New(db, cfg.Crawler, autoLinker)
	crawlerSvc.RegisterParser(crawler.NewViperGirls())
	crawlerSvc.RegisterParser(crawler.NewJKForum())
	crawlerSvc.RegisterParser(crawler.NewKittyKats())
	defer crawlerSvc.Stop()

	// Build a shared HTTP client and ripper registry.
	httpClient := utils.NewHTTPClient(utils.WithTimeout(cfg.Crawler.RequestTimeout))
	ripperReg := ripper.NewRegistry(
		cfg.Storage.ImagesDir,
		httpClient,
		ripper.WithUserAgent(cfg.Crawler.UserAgent),
	)
	ripperproviders.RegisterAll(ripperReg, httpClient, cfg.Crawler.UserAgent)

	// Initialise WireGuard VPN for age-gated provider APIs.
	vpnSvc := vpn.New(cfg.WireGuard, httpClient)

	// Build a VPN-routed client for providers that need it (MetArt, Playboy, etc.).
	// We use a representative URL to get the right client type.
	vpnClient := vpnSvc.GetHTTPClient("https://www.metart.com")

	stashDBKey := cfg.Providers.StashDBAPIKey
	if stashDBKey == "" {
		slog.Warn("config.providers.stashdb_api_key not set — StashDB searches will fail (authentication required)")
	} else {
		slog.Info("StashDB API key loaded from config", "key_length", len(stashDBKey))
	}
	enricher := providers.NewEnricher(httpClient, cfg.Crawler.UserAgent, stashDBKey, vpnClient)

	thumbWorker := workers.NewThumbnailWorker(db, cfg.Storage.ImagesDir, cfg.Storage.ThumbnailsDir)
	colorWorker := workers.NewColorWorker(db, cfg.Storage.ImagesDir)

	// Video ripper registry — separate from image rippers, saves to videos dir.
	videoReg := video.NewRegistry(cfg.Storage.VideosDir, httpClient, cfg.Crawler.UserAgent)
	videoReg.Register(video.NewTnaFlixRipper(httpClient, cfg.Crawler.UserAgent))
	videoReg.Register(video.NewYtDlpRipper("")) // no cookies file for now
	videoReg.Register(video.NewPMVHavenRipper(httpClient, cfg.Crawler.UserAgent))

	videoWorker := workers.NewVideoWorker(db, cfg.Storage.VideosDir, cfg.Storage.ThumbnailsDir)
	trickplayWorker := workers.NewTrickplayWorker(db, cfg.Storage.VideosDir, cfg.Storage.ThumbnailsDir)

	dbWriter := database.NewWriter(db)
	defer dbWriter.Stop()

	// Create separate worker pools for images, videos, and crawls.
	imageQueue := queue.New(db, dbWriter, cfg.Queue.Workers, cfg.Crawler.MaxRetries)
	imageQueue.SetTypeFilter([]models.QueueType{models.QueueTypeImage, models.QueueTypeGallery})
	imageQueue.SetProviderLimit(cfg.Queue.ProviderLimit)
	imageQueue.SetProviderPool(cfg.Queue.ProviderPool)

	videoQueue := queue.New(db, dbWriter, cfg.Queue.VideoWorkers, cfg.Crawler.MaxRetries)
	videoQueue.SetTypeFilter([]models.QueueType{models.QueueTypeVideo})
	videoQueue.SetProviderLimit(1) // Usually one video at a time per host is better
	videoQueue.SetProviderPool(2)

	crawlQueue := queue.New(db, dbWriter, cfg.Queue.CrawlWorkers, cfg.Crawler.MaxRetries)
	crawlQueue.SetTypeFilter([]models.QueueType{models.QueueTypeCrawl})
	crawlQueue.SetProviderLimit(cfg.Queue.CrawlWorkers)
	crawlQueue.SetProviderPool(cfg.Queue.CrawlWorkers)

	queueMgr := queue.NewGroup(imageQueue, videoQueue, crawlQueue)
	processors.New(db, dbWriter, ripperReg, *cfg, thumbWorker, colorWorker, videoReg, videoWorker, trickplayWorker).Register(queueMgr)

	// WebSocket hub for real-time download progress.
	wsHub := ws.NewHub()
	go wsHub.Run()
	statusTracker := ws.NewStatusTracker(wsHub)
	queueMgr.SetStatusTracker(statusTracker)

	queueMgr.Start()
	defer queueMgr.Stop()

	shutdownRequests := make(chan string, 1)
	requestShutdown := func(reason string) bool {
		select {
		case shutdownRequests <- reason:
			return true
		default:
			return false
		}
	}

	// Gallery metadata service — uses VPN-aware client for age-gated provider APIs.
	metadataSvc := providers.NewGalleryMetadataService(vpnSvc.GetHTTPClient, cfg.Crawler.UserAgent)

	// Create photoDownloader for use in both router and async startup scan
	photoDownloader := personphoto.NewDownloader(cfg.Storage.PersonPhotosDir, httpClient, "")

	router := buildRouter(db, crawlerSvc, queueMgr, autoLinker, enricher, cfg.Storage, wsHub, metadataSvc, httpClient, photoDownloader, requestShutdown)

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start the server in a goroutine so we can listen for shutdown signals.
	go func() {
		slog.Info("server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Run missing file scans asynchronously so the API is available immediately.
	go runMissingFileScans(db, cfg, photoDownloader, enricher)

	// Block until a termination signal is received.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	stopReason := "termination signal"
	select {
	case sig := <-quit:
		stopReason = "signal: " + sig.String()
	case reason := <-shutdownRequests:
		stopReason = reason
	}

	slog.Warn("shutting down server...", "reason", stopReason)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

type queueManager interface {
	Pause()
	Resume()
	IsPaused() bool
	ActiveDownloads() []queue.ActiveDownload
}

// buildRouter wires up all routes and returns the configured gin.Engine.
func buildRouter(db *database.DB, crawlerSvc *crawler.Crawler, queueMgr queueManager, al *linker.AutoLinker, enricher *providers.Enricher, storage config.StorageConfig, wsHub *ws.Hub, metadataSvc *providers.GalleryMetadataService, httpClient *http.Client, photoDownloader *personphoto.Downloader, requestShutdown func(reason string) bool) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())

	v1 := r.Group("/api/v1")

	handlers.NewSourceHandler(db, crawlerSvc).RegisterRoutes(v1.Group("/sources"))
	handlers.NewGalleryHandler(db, storage, metadataSvc, al).RegisterRoutes(v1.Group("/galleries"))
	handlers.NewImageHandler(db, storage).RegisterRoutes(v1.Group("/images"))
	handlers.NewVideoHandler(db).RegisterRoutes(v1.Group("/videos"))
	handlers.NewPeopleHandler(db, al, enricher, photoDownloader).RegisterRoutes(v1.Group("/people"))
	handlers.NewAdminHandler(db, crawlerSvc, queueMgr, al, requestShutdown).RegisterRoutes(v1.Group("/admin"))

	// Health check endpoint.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// WebSocket endpoint for real-time download progress.
	r.GET("/ws", wsHub.HandleWebSocket)

	// Serve downloaded media files (images, thumbnails, videos) from the
	// data/ directory so the frontend can display them via /data/images/...
	r.Static("/data/images", "data/images")
	r.Static("/data/thumbnails", "data/thumbnails")
	r.Static("/data/videos", "data/videos")
	r.Static("/data/person_photos", storage.PersonPhotosDir)

	// Serve the React SPA from web/dist/ when available.
	// Any request that does not match an API route or a static file is
	// served index.html so the client-side router can handle it.
	distDir := "web/dist"
	if info, err := os.Stat(distDir); err == nil && info.IsDir() {
		slog.Info("serving frontend", "dir", distDir)
		staticFS := http.Dir(distDir)

		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path

			// Skip API paths — they should 404 normally.
			if strings.HasPrefix(path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}

			// Try to serve the requested file directly (JS, CSS, images, etc.).
			clean := filepath.Clean(path)
			if f, err := staticFS.Open(clean); err == nil {
				defer f.Close()
				if stat, err := f.Stat(); err == nil && !stat.IsDir() {
					http.ServeFile(c.Writer, c.Request, filepath.Join(distDir, clean))
					return
				}
			}

			// SPA fallback — serve index.html for all other paths.
			c.File(filepath.Join(distDir, "index.html"))
		})
	}

	return r
}

// setupLogger configures the global slog logger based on config.
func setupLogger(cfg config.LogConfig) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

func runMissingFileScans(db *database.DB, cfg *config.Config, photoDownloader *personphoto.Downloader, enricher *providers.Enricher) {
	slog.Warn("startup: beginning missing file scans")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Prioritize person-photo recovery so profile images are restored quickly
	// and are not blocked behind large image queue scans.
	scanMissingPersonPhotos(ctx, db, cfg, photoDownloader, enricher)
	scanMissingImages(ctx, db, cfg)
	slog.Warn("startup: missing file scans finished")
}

func scanMissingImages(ctx context.Context, db *database.DB, cfg *config.Config) {
	slog.Warn("startup: beginning image file restore scan")
	favorite := true
	nonFavorite := false

	favoriteImages, err := db.ListImages(ctx, database.ImageFilter{IsFavorite: &favorite, Limit: -1})
	if err != nil {
		slog.Error("startup: failed to list favorite images for missing file check", "error", err)
		return
	}

	regularImages, err := db.ListImages(ctx, database.ImageFilter{IsFavorite: &nonFavorite, Limit: -1})
	if err != nil {
		slog.Error("startup: failed to list regular images for missing file check", "error", err)
		return
	}

	missingCount := 0
	queueFailures := 0
	pendingOrActiveSkipped := 0

	processMissing := func(imgs []models.Image, priorityLog bool) {
		for _, img := range imgs {
			if img.Filename == "" {
				continue
			}
			imgPath := filepath.Join(cfg.Storage.ImagesDir, img.Filename)
			if _, err := os.Stat(imgPath); err == nil {
				continue
			}

			slog.Debug("found missing image", "image_id", img.ID, "file", img.Filename, "is_favorite", img.IsFavorite)

			if img.OriginalURL == nil || *img.OriginalURL == "" {
				slog.Warn("image missing but has no OriginalURL, cannot requeue", "image_id", img.ID, "file", img.Filename)
				continue
			}
			existingItem, err := db.GetQueueItemByTarget(ctx, img.ID, "image")
			if err == nil && existingItem != nil {
				if existingItem.Status == string(models.QueueStatusCompleted) {
					if err := db.DeleteQueueItem(ctx, existingItem.ID); err != nil {
						slog.Error("failed to delete completed item", "image_id", img.ID, "queue_id", existingItem.ID, "error", err)
						queueFailures++
						continue
					}
					slog.Info("deleted completed item, will re-queue", "image_id", img.ID, "queue_id", existingItem.ID, "priority_favorite", priorityLog)
				} else if existingItem.Status == string(models.QueueStatusPending) || existingItem.Status == string(models.QueueStatusActive) {
					pendingOrActiveSkipped++
					continue
				} else {
					if err := db.UpdateQueueStatus(ctx, existingItem.ID, models.QueueStatusPending, nil); err != nil {
						slog.Error("failed to move item to pending", "image_id", img.ID, "queue_id", existingItem.ID, "error", err)
						queueFailures++
						continue
					}
					slog.Info("moved item to pending", "image_id", img.ID, "queue_id", existingItem.ID, "previous_status", existingItem.Status, "priority_favorite", priorityLog)
					missingCount++
					continue
				}
			}
			if !errors.Is(err, database.ErrNotFound) && err != nil {
				slog.Error("failed to check existing queue item", "image_id", img.ID, "error", err)
				continue
			}
			job := &models.DownloadQueue{
				Type:     "image",
				URL:      *img.OriginalURL,
				TargetID: &img.ID,
			}
			err = db.EnqueueItem(ctx, job)
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
					slog.Info("already queued for download (skipped duplicate)", "image_id", img.ID, "file", img.Filename, "priority_favorite", priorityLog)
				} else {
					slog.Error("failed to enqueue missing image for download", "image_id", img.ID, "error", err, "priority_favorite", priorityLog)
					queueFailures++
				}
			} else {
				slog.Info("enqueued missing image for download", "image_id", img.ID, "file", img.Filename, "priority_favorite", priorityLog)
				missingCount++
			}
		}
	}

	slog.Info("missing image scan", "favorites", len(favoriteImages), "regulars", len(regularImages))

	processMissing(favoriteImages, true)
	processMissing(regularImages, false)

	slog.Warn("startup: image file restore scan complete", "missing_found", missingCount, "pending_or_active_skipped", pendingOrActiveSkipped, "queue_failures", queueFailures)
}

func scanMissingPersonPhotos(ctx context.Context, db *database.DB, cfg *config.Config, photoDownloader *personphoto.Downloader, enricher *providers.Enricher) {
	slog.Warn("startup: beginning person photo restore scan")
	people, err := db.ListPeople(ctx, database.PeopleFilter{Limit: -1})
	if err != nil {
		slog.Error("startup: failed to list people for missing photo check", "error", err)
		return
	}

	missingPhotoCount := 0
	peopleNeedingRestore := 0
	noURLsCount := 0
	restoreAttemptCount := 0
	restoreFailedCount := 0
	for _, p := range people {
		photoPaths := decodePhotoPaths(p.Photos)
		needsRestore := len(photoPaths) == 0
		if !needsRestore {
			for _, photoPath := range photoPaths {
				fullPath := filepath.Join(cfg.Storage.PersonPhotosDir, strings.TrimPrefix(photoPath, "/data/person_photos/"))
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					needsRestore = true
					break
				}
			}
		}
		if !needsRestore {
			continue
		}
		peopleNeedingRestore++
		slog.Info("startup person photo restore needed", "person_id", p.ID, "name", p.Name, "stored_photo_paths", len(photoPaths))

		photoURLs, err := db.GetPersonPhotoURLs(ctx, p.ID)
		if err != nil {
			slog.Warn("failed to load stored person photo URLs", "person_id", p.ID, "name", p.Name, "error", err)
		}
		slog.Info("startup person photo URL state", "person_id", p.ID, "name", p.Name, "stored_url_count", len(photoURLs))
		if len(photoURLs) == 0 {
			photoURLs = recoverPersonPhotoURLs(ctx, db, enricher, p)
			if len(photoURLs) > 0 {
				if err := db.SavePersonPhotoURLs(ctx, p.ID, photoURLs); err != nil {
					slog.Warn("failed to persist recovered person photo URLs", "person_id", p.ID, "name", p.Name, "error", err)
				}
				slog.Info("startup person photo URLs recovered", "person_id", p.ID, "name", p.Name, "recovered_url_count", len(photoURLs))
			}
		}
		if len(photoURLs) == 0 {
			noURLsCount++
			slog.Warn("person needs photo restore but no photo URLs were found", "person_id", p.ID, "name", p.Name)
			continue
		}
		restoreAttemptCount++

		downloadedPaths := photoDownloader.DownloadAll(ctx, photoURLs, p.ID)
		if len(downloadedPaths) > 0 {
			merged := mergePhotoPaths(decodePhotoPaths(p.Photos), downloadedPaths)
			encoded, err := json.Marshal(merged)
			if err == nil {
				s := string(encoded)
				p.Photos = &s
				if err := db.UpdatePerson(ctx, &p); err != nil {
					slog.Error("failed to update person photos", "person_id", p.ID, "error", err)
				} else {
					slog.Info("re-downloaded person photos", "person_id", p.ID, "name", p.Name, "count", len(downloadedPaths))
					missingPhotoCount++
				}
			}
		} else {
			restoreFailedCount++
			slog.Warn("startup person photo restore attempted but no photos downloaded", "person_id", p.ID, "name", p.Name, "url_count", len(photoURLs))
		}
	}
	slog.Warn("startup: person photo restore scan complete",
		"people_total", len(people),
		"people_needing_restore", peopleNeedingRestore,
		"restore_attempts", restoreAttemptCount,
		"people_photos_restored", missingPhotoCount,
		"restore_attempts_without_downloads", restoreFailedCount,
		"missing_url_sources", noURLsCount,
	)
}

func recoverPersonPhotoURLs(ctx context.Context, db *database.DB, enricher *providers.Enricher, p models.Person) []string {
	if enricher == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var recovered []string

	addURLs := func(urls []string) {
		for _, u := range urls {
			if u == "" {
				continue
			}
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			recovered = append(recovered, u)
		}
	}
	addInfoURLs := func(info *providers.PersonInfo) {
		if info == nil {
			return
		}
		addURLs(info.ImageURLs)
		if info.ImageURL != nil && *info.ImageURL != "" {
			addURLs([]string{*info.ImageURL})
		}
	}

	ids, err := db.ListPersonIdentifiers(ctx, p.ID)
	if err != nil {
		slog.Warn("failed to list person identifiers for photo URL recovery", "person_id", p.ID, "name", p.Name, "error", err)
	} else {
		if len(ids) > 0 {
			slog.Info("startup person photo recovery: trying identifiers", "person_id", p.ID, "name", p.Name, "identifier_count", len(ids))
			// If we have provider IDs, recover photos the same way as first-time identify:
			// fetch provider records by external ID and use returned image URLs.
			for _, ident := range ids {
				info, lookupErr := enricher.GetByExternalID(ctx, ident.Provider, ident.ExternalID)
				if lookupErr != nil {
					slog.Debug("startup person photo recovery: identifier lookup failed", "person_id", p.ID, "name", p.Name, "provider", ident.Provider, "external_id", ident.ExternalID, "error", lookupErr)
					continue
				}
				addInfoURLs(info)
				if len(recovered) > 0 {
					slog.Info("recovered person photo URLs from identifier lookup", "person_id", p.ID, "name", p.Name, "provider", ident.Provider, "count", len(recovered))
					return recovered
				}
			}
			slog.Warn("person has identifiers but no photo URLs recovered from provider ID lookups", "person_id", p.ID, "name", p.Name, "identifier_count", len(ids))
			return nil
		}
	}

	slog.Info("startup person photo recovery: no identifiers, trying name lookup", "person_id", p.ID, "name", p.Name)
	byName := enricher.LookupPerson(ctx, p.Name)
	addInfoURLs(&byName.Merged)
	if len(recovered) > 0 {
		slog.Info("recovered person photo URLs from name lookup", "person_id", p.ID, "name", p.Name, "count", len(recovered))
	}
	return recovered
}

func decodePhotoPaths(photos *string) []string {
	if photos == nil || *photos == "" {
		return nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(*photos), &paths); err != nil {
		return nil
	}
	return paths
}

func mergePhotoPaths(existing, newPaths []string) []string {
	seen := make(map[string]struct{}, len(existing))
	merged := make([]string, 0, len(existing)+len(newPaths))
	for _, p := range existing {
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	for _, p := range newPaths {
		if _, ok := seen[p]; !ok {
			merged = append(merged, p)
			seen[p] = struct{}{}
		}
	}
	return merged
}
