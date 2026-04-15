// Package database — image queries.
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/carlj/godownload/internal/models"
)

// ImageFilter holds optional filter parameters for ListImages.
type ImageFilter struct {
	GalleryID  *int64
	IsVideo    *bool
	IsFavorite *bool
	Limit      int
	Offset     int
	SortBy     string // "newest" (default), "oldest", "largest", "smallest", "random"
	RandomSeed int64  // seed for random ordering
}

const (
	SortByNewest   = "newest"
	SortByOldest   = "oldest"
	SortByLargest  = "largest"
	SortBySmallest = "smallest"
	SortByRandom   = "random"
)

// ListImages returns a paginated list of images.
func (db *DB) ListImages(ctx context.Context, f ImageFilter) ([]models.Image, error) {
	// Use Limit = -1 to get all images (no pagination)
	if f.Limit < 0 {
		f.Limit = 1000000 // arbitrarily large, will fit in memory
	} else if f.Limit == 0 {
		f.Limit = 50
	}

	// Default sort: newest first (gallery created_at DESC, image created_at DESC)
	sortOrder := "COALESCE(galleries.created_at, images.created_at) DESC, images.created_at DESC"
	switch f.SortBy {
	case SortByOldest:
		sortOrder = "COALESCE(galleries.created_at, images.created_at) ASC, images.created_at ASC"
	case SortByLargest:
		sortOrder = "COALESCE(galleries.created_at, images.created_at) DESC, (images.width * images.height) DESC"
	case SortBySmallest:
		sortOrder = "COALESCE(galleries.created_at, images.created_at) DESC, (images.width * images.height) ASC"
	case SortByRandom:
		sortOrder = "ABS((images.id * ?) % 1000000) DESC"
	}

	query := `SELECT images.id, images.gallery_id, images.filename, images.original_url,
	                 images.width, images.height, images.duration_seconds,
	                 images.file_hash, images.dominant_colors,
	                 images.is_video, images.vr_mode, images.is_favorite, images.created_at
	            FROM images
	       LEFT JOIN galleries ON images.gallery_id = galleries.id
	            WHERE 1=1`
	args := []any{}

	if f.GalleryID != nil {
		query += " AND gallery_id = ?"
		args = append(args, *f.GalleryID)
	}
	if f.IsVideo != nil {
		query += " AND is_video = ?"
		args = append(args, *f.IsVideo)
	}
	if f.IsFavorite != nil {
		query += " AND is_favorite = ?"
		args = append(args, *f.IsFavorite)
	}

	query += " ORDER BY " + sortOrder + " LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	images := []models.Image{}
	if err := db.SelectContext(ctx, &images, query, args...); err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	return images, nil
}

// GetImage retrieves a single image by ID.
func (db *DB) GetImage(ctx context.Context, id int64) (*models.Image, error) {
	var img models.Image
	err := db.GetContext(ctx, &img,
		`SELECT id, gallery_id, filename, original_url, width, height,
		        duration_seconds, file_hash, dominant_colors,
		        is_video, vr_mode, is_favorite, created_at
		   FROM images WHERE id = ?`, id,
	)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting image %d: %w", id, err)
	}
	return &img, nil
}

// CreateImage inserts a new image record and populates ID and CreatedAt.
func (db *DB) CreateImage(ctx context.Context, img *models.Image) error {
	var result sql.Result
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		result, err = db.ExecContext(ctx,
			`INSERT INTO images
			   (gallery_id, filename, original_url, width, height, duration_seconds,
			    file_hash, dominant_colors, is_video, vr_mode, is_favorite)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			img.GalleryID, img.Filename, img.OriginalURL,
			img.Width, img.Height, img.DurationSeconds,
			img.FileHash, img.DominantColors,
			img.IsVideo, img.VRMode, img.IsFavorite,
		)
		if err == nil {
			break
		}
		// Retry on locked database
		if !strings.Contains(err.Error(), "database is locked") {
			break
		}
		slog.Warn("database: CreateImage retrying", "attempt", attempt+1, "error", err)
		time.Sleep(time.Duration(attempt*100) * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("creating image: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting image id: %w", err)
	}
	img.ID = id
	img.CreatedAt = time.Now().UTC()
	return nil
}

// DeleteImage removes an image by ID.
func (db *DB) DeleteImage(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM images WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting image %d: %w", id, err)
	}
	return nil
}

// FindImageByGalleryAndFilename finds an existing image by gallery_id and filename.
// Returns ErrNotFound if no matching image exists.
func (db *DB) FindImageByGalleryAndFilename(ctx context.Context, galleryID *int64, filename string) (*models.Image, error) {
	var img models.Image
	err := db.GetContext(ctx, &img,
		`SELECT id, gallery_id, filename, original_url, width, height,
		        duration_seconds, file_hash, dominant_colors, is_video, vr_mode,
		        is_favorite, created_at
		   FROM images
		  WHERE gallery_id IS ? AND filename = ?`,
		galleryID, filename,
	)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("finding image: %w", err)
	}
	return &img, nil
}

// UpdateImageFilename updates the filename for an image.
func (db *DB) UpdateImageFilename(ctx context.Context, id int64, filename string) error {
	_, err := db.ExecContext(ctx, `UPDATE images SET filename = ? WHERE id = ?`, filename, id)
	if err != nil {
		return fmt.Errorf("updating filename for image %d: %w", id, err)
	}
	return nil
}

// UpdateImageVideoMeta sets the width, height, and duration for a video image record.
func (db *DB) UpdateImageVideoMeta(ctx context.Context, id int64, width, height, durationSeconds int) error {
	_, err := db.ExecContext(ctx,
		`UPDATE images SET width = ?, height = ?, duration_seconds = ? WHERE id = ?`,
		width, height, durationSeconds, id,
	)
	if err != nil {
		return fmt.Errorf("updating video metadata for image %d: %w", id, err)
	}
	return nil
}

// ToggleFavorite flips is_favorite for the given image and returns the new state.
func (db *DB) ToggleFavorite(ctx context.Context, id int64) (bool, error) {
	_, err := db.ExecContext(ctx,
		`UPDATE images SET is_favorite = NOT is_favorite WHERE id = ?`, id,
	)
	if err != nil {
		return false, fmt.Errorf("toggling favorite on image %d: %w", id, err)
	}

	var isFavorite bool
	if err := db.GetContext(ctx, &isFavorite,
		`SELECT is_favorite FROM images WHERE id = ?`, id,
	); err != nil {
		return false, fmt.Errorf("reading favorite state for image %d: %w", id, err)
	}

	return isFavorite, nil
}

// CountImages returns the total number of images matching a filter (ignoring Limit/Offset).
func (db *DB) CountImages(ctx context.Context, f ImageFilter) (int64, error) {
	query := `SELECT COUNT(*) FROM images WHERE 1=1`
	args := []any{}

	if f.GalleryID != nil {
		query += " AND gallery_id = ?"
		args = append(args, *f.GalleryID)
	}
	if f.IsVideo != nil {
		query += " AND is_video = ?"
		args = append(args, *f.IsVideo)
	}
	if f.IsFavorite != nil {
		query += " AND is_favorite = ?"
		args = append(args, *f.IsFavorite)
	}

	var count int64
	if err := db.GetContext(ctx, &count, query, args...); err != nil {
		return 0, fmt.Errorf("counting images: %w", err)
	}
	return count, nil
}

// ColorSearchResult pairs an image with its distance from the search color.
type ColorSearchResult struct {
	Image    models.Image `json:"image"`
	Distance float64      `json:"distance"`
}

// SearchImagesByColor finds images whose dominant colors are close to the given
// hex color string (e.g. "#ff0000"). It loads all images with non-null
// dominant_colors, computes the minimum squared Euclidean distance in RGB space
// between the search color and each image's palette, then returns the closest
// matches sorted by distance.
func (db *DB) SearchImagesByColor(ctx context.Context, hexColor string, maxDist float64, limit int) ([]ColorSearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	r, g, b, err := parseHexColor(hexColor)
	if err != nil {
		return nil, fmt.Errorf("parsing color %q: %w", hexColor, err)
	}

	images := []models.Image{}
	if err := db.SelectContext(ctx, &images,
		`SELECT id, gallery_id, filename, original_url, width, height,
		        duration_seconds, file_hash, dominant_colors,
		        is_video, vr_mode, is_favorite, created_at
		   FROM images
		  WHERE dominant_colors IS NOT NULL AND is_video = 0`,
	); err != nil {
		return nil, fmt.Errorf("fetching images for color search: %w", err)
	}

	var results []ColorSearchResult
	for _, img := range images {
		if img.DominantColors == nil {
			continue
		}

		var hexColors []string
		if err := json.Unmarshal([]byte(*img.DominantColors), &hexColors); err != nil {
			continue // skip malformed entries
		}

		minDist := math.MaxFloat64
		for _, hc := range hexColors {
			cr, cg, cb, parseErr := parseHexColor(hc)
			if parseErr != nil {
				continue
			}
			d := rgbDistSq(r, g, b, cr, cg, cb)
			if d < minDist {
				minDist = d
			}
		}

		if maxDist > 0 && minDist > maxDist {
			continue
		}

		results = append(results, ColorSearchResult{
			Image:    img,
			Distance: minDist,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// parseHexColor parses a hex color string like "#ff0000" or "ff0000" into RGB components.
func parseHexColor(s string) (r, g, b float64, err error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex color length: %q", s)
	}

	var ri, gi, bi uint8
	_, err = fmt.Sscanf(s, "%02x%02x%02x", &ri, &gi, &bi)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parsing hex color %q: %w", s, err)
	}
	return float64(ri), float64(gi), float64(bi), nil
}

// rgbDistSq returns the squared Euclidean distance between two RGB colors.
func rgbDistSq(r1, g1, b1, r2, g2, b2 float64) float64 {
	dr := r1 - r2
	dg := g1 - g2
	db := b1 - b2
	return dr*dr + dg*dg + db*db
}
