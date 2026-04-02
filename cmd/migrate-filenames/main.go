package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	_ "modernc.org/sqlite"
)

func main() {
	imagesDir := flag.String("images-dir", "data/images", "Directory where images are stored")
	thumbnailsDir := flag.String("thumbnails-dir", "data/thumbnails", "Directory where thumbnails are stored")
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	dryRun := flag.Bool("dry-run", false, "If true, only print what would be done without making changes")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	// Override storage dirs from flags (or config)
	if *imagesDir != "" {
		cfg.Storage.ImagesDir = *imagesDir
	}
	if *thumbnailsDir != "" {
		cfg.Storage.ThumbnailsDir = *thumbnailsDir
	}

	// Open database
	db, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()

	// Get all images
	images, err := db.ListImages(ctx, database.ImageFilter{Limit: -1}) // -1 = no limit
	if err != nil {
		slog.Error("listing images", "error", err)
		os.Exit(1)
	}

	slog.Info("found images", "count", len(images))

	// Build a map of old filename -> new filename for gallery thumbnail updates
	filenameMap := make(map[string]string)

	processed := 0
	renamed := 0
	skipped := 0
	errors := 0

	for _, img := range images {
		oldFilename := img.Filename

		// Skip if already using hash format (64 char hex + extension)
		if isHashFilename(oldFilename) {
			slog.Debug("skipping (already hash-based)", "id", img.ID, "filename", oldFilename)
			skipped++
			continue
		}

		// Build full path to existing file
		oldPath := filepath.Join(cfg.Storage.ImagesDir, oldFilename)
		newFilename := ""
		newPath := ""

		// Check if file exists
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			slog.Warn("file not found on disk", "id", img.ID, "path", oldPath)
			// Still try to compute hash from DB if we have one, otherwise skip
			if img.FileHash != nil && *img.FileHash != "" {
				ext := extensionFromFilename(oldFilename)
				newFilename = *img.FileHash + ext
				newPath = filepath.Join(cfg.Storage.ImagesDir, newFilename)
			} else {
				slog.Error("no file on disk and no hash in DB - cannot migrate", "id", img.ID)
				errors++
				continue
			}
		} else {
			// Compute hash from file
			hash, err := computeHash(oldPath)
			if err != nil {
				slog.Error("computing hash", "id", img.ID, "error", err)
				errors++
				continue
			}

			ext := extensionFromFilename(oldFilename)
			newFilename = hash + ext
			newPath = filepath.Join(cfg.Storage.ImagesDir, newFilename)

			// Check if target already exists
			if _, err := os.Stat(newPath); err == nil {
				// File already exists - same content (collision), don't rename but update DB
				slog.Info("collision: same content exists", "id", img.ID, "new_filename", newFilename)
			} else if !os.IsNotExist(err) {
				slog.Error("checking target file", "id", img.ID, "error", err)
				errors++
				continue
			} else {
				// Target doesn't exist - rename the file
				if !*dryRun {
					if err := os.Rename(oldPath, newPath); err != nil {
						slog.Error("renaming file", "id", img.ID, "from", oldPath, "to", newPath, "error", err)
						errors++
						continue
					}
					slog.Debug("renamed file", "id", img.ID, "from", oldFilename, "to", newFilename)
				} else {
					slog.Info("DRY RUN: would rename", "id", img.ID, "from", oldFilename, "to", newFilename)
				}
				renamed++
			}

			// Handle thumbnail
			oldThumb := filepath.Join(cfg.Storage.ThumbnailsDir, thumbnailName(oldFilename))
			newThumb := filepath.Join(cfg.Storage.ThumbnailsDir, thumbnailName(newFilename))

			if _, err := os.Stat(oldThumb); err == nil {
				if _, err := os.Stat(newThumb); os.IsNotExist(err) {
					if !*dryRun {
						if err := os.Rename(oldThumb, newThumb); err != nil {
							slog.Warn("renaming thumbnail", "id", img.ID, "error", err)
						}
					} else {
						slog.Info("DRY RUN: would rename thumbnail", "id", img.ID)
					}
				}
			}
		}

		// Store mapping for gallery thumbnail updates
		filenameMap[oldFilename] = newFilename

		// Update database
		if !*dryRun {
			if err := db.UpdateImageFilename(ctx, img.ID, newFilename); err != nil {
				slog.Error("updating filename in DB", "id", img.ID, "error", err)
				errors++
				continue
			}
		}

		processed++
		if processed%100 == 0 {
			slog.Info("progress", "processed", processed, "renamed", renamed, "skipped", skipped, "errors", errors)
		}
	}

	slog.Info("migration complete (images)",
		"total", len(images),
		"processed", processed,
		"renamed", renamed,
		"skipped", skipped,
		"errors", errors,
	)

	// ---------------------------------------------------------------------
	// Migrate gallery thumbnails (local_thumbnail_path in galleries table)
	// ---------------------------------------------------------------------
	slog.Info("migrating gallery thumbnails...")

	galleries, err := db.ListGalleries(ctx, database.GalleryFilter{Limit: -1})
	if err != nil {
		slog.Error("listing galleries", "error", err)
		os.Exit(1)
	}

	galleryThumbsUpdated := 0
	for _, g := range galleries {
		if g.LocalThumbnailPath == nil || *g.LocalThumbnailPath == "" {
			continue
		}

		oldThumbPath := *g.LocalThumbnailPath

		// Find the corresponding image filename
		// The local_thumbnail_path is like "imagename_thumb.jpg"
		// We need to find the original image filename and map it
		var oldImageFilename string
		for oldImg, _ := range filenameMap {
			if thumbnailName(oldImg) == oldThumbPath {
				oldImageFilename = oldImg
				break
			}
		}

		if oldImageFilename == "" {
			slog.Debug("gallery thumbnail not from known images", "gallery_id", g.ID, "thumb", oldThumbPath)
			continue
		}

		newImageFilename := filenameMap[oldImageFilename]
		if newImageFilename == "" {
			slog.Warn("no new filename mapping found", "gallery_id", g.ID, "old_image", oldImageFilename)
			continue
		}

		newThumbPath := thumbnailName(newImageFilename)

		// Update gallery thumbnail path in database
		if !*dryRun {
			if err := db.SetGalleryThumbnail(ctx, g.ID, newThumbPath); err != nil {
				slog.Error("updating gallery thumbnail", "gallery_id", g.ID, "error", err)
				continue
			}
		} else {
			slog.Info("DRY RUN: would update gallery thumbnail", "gallery_id", g.ID, "from", oldThumbPath, "to", newThumbPath)
		}
		galleryThumbsUpdated++
	}

	slog.Info("migration complete",
		"total_galleries", len(galleries),
		"gallery_thumbnails_updated", galleryThumbsUpdated,
	)

	if *dryRun {
		slog.Info("DRY RUN - no changes made")
	}
}

// isHashFilename returns true if filename appears to be a hash-based name
// (64 char hex string + extension)
func isHashFilename(filename string) bool {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	if len(name) != 64 {
		return false
	}
	for _, c := range name {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// computeHash computes SHA-256 hash of a file
func computeHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// extensionFromFilename extracts the extension from a filename
func extensionFromFilename(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		return ".jpg"
	}
	return ext
}

// thumbnailName returns the thumbnail filename for a given image filename
func thumbnailName(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return base + "_thumb.jpg"
}
