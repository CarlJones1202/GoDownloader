// Package database — image queries.
package database

import (
	"context"
	"fmt"
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
}

// ListImages returns a paginated list of images.
func (db *DB) ListImages(ctx context.Context, f ImageFilter) ([]models.Image, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := `SELECT id, gallery_id, filename, original_url, width, height,
	                 duration_seconds, file_hash, dominant_colors,
	                 is_video, vr_mode, is_favorite, created_at
	            FROM images WHERE 1=1`
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

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
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
	result, err := db.ExecContext(ctx,
		`INSERT INTO images
		   (gallery_id, filename, original_url, width, height, duration_seconds,
		    file_hash, dominant_colors, is_video, vr_mode, is_favorite)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		img.GalleryID, img.Filename, img.OriginalURL,
		img.Width, img.Height, img.DurationSeconds,
		img.FileHash, img.DominantColors,
		img.IsVideo, img.VRMode, img.IsFavorite,
	)
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
