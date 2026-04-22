package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	deleteFiles := flag.Bool("delete-files", false, "If true, also deletes image and video files from disk")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	// Open database
	db, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Wipe DB records
	slog.Info("wiping gallery, image, and queue records from database...")
	if err := wipeDB(ctx, db); err != nil {
		slog.Error("failed to wipe database", "error", err)
		os.Exit(1)
	}

	// 2. Delete physical files if requested
	if *deleteFiles {
		slog.Info("deleting physical files from storage...")
		if err := deletePhysicalFiles(cfg); err != nil {
			slog.Error("failed to delete physical files", "error", err)
			// we continue to re-queue even if file deletion partially fails
		}
	}

	// 3. Re-queue all enabled sources
	slog.Info("re-queuing enabled sources...")
	sources, err := db.ListSources(ctx)
	if err != nil {
		slog.Error("failed to list sources", "error", err)
		os.Exit(1)
	}

	requeued := 0
	for _, s := range sources {
		if !s.Enabled {
			slog.Debug("skipping disabled source", "id", s.ID, "name", s.Name)
			continue
		}

		item := &models.DownloadQueue{
			Type:     string(models.QueueTypeCrawl),
			URL:      s.URL,
			TargetID: &s.ID,
		}

		if err := db.EnqueueItem(ctx, item); err != nil {
			slog.Error("failed to enqueue source", "id", s.ID, "url", s.URL, "error", err)
			continue
		}
		requeued++
	}

	slog.Info("reset and re-queue complete", "sources_requeued", requeued)
}

// wipeDB clears galleries, images, tags, and the queue within a single transaction.
func wipeDB(ctx context.Context, db *database.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Order matters for foreign keys, though CASCADE/SET NULL handles some of it.
	tables := []string{
		"image_tags",
		"gallery_persons",
		"images",
		"galleries",
		"download_queue",
	}

	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("deleting from %s: %w", table, err)
		}
		// Reset autoincrement counters
		_, _ = tx.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name = ?", table)
	}

	// Reset source last_crawled_at so they appear fresh
	if _, err := tx.ExecContext(ctx, "UPDATE sources SET last_crawled_at = NULL"); err != nil {
		return fmt.Errorf("resetting sources last_crawled_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// deletePhysicalFiles removes files from images, thumbnails, and videos directories.
func deletePhysicalFiles(cfg *config.Config) error {
	dirs := []string{
		cfg.Storage.ImagesDir,
		cfg.Storage.ThumbnailsDir,
		cfg.Storage.VideosDir,
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			slog.Warn("could not read directory", "path", dir, "error", err)
			continue
		}

		deleted := 0
		for _, entry := range entries {
			// Skip hidden files or common project files if any
			if entry.Name() == ".gitkeep" || entry.Name() == ".gitignore" {
				continue
			}

			fullPath := filepath.Join(dir, entry.Name())
			if err := os.RemoveAll(fullPath); err != nil {
				slog.Warn("failed to delete path", "path", fullPath, "error", err)
			} else {
				deleted++
			}
		}
		slog.Info("cleared directory", "path", dir, "files_deleted", deleted)
	}

	return nil
}
