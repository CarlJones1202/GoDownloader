// Package database — gallery queries.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/carlj/godownload/internal/models"
)

// GalleryFilter holds optional filter parameters for ListGalleries.
type GalleryFilter struct {
	SourceID *int64
	Provider *string
	Search   *string // title LIKE
	Limit    int
	Offset   int
}

// ListGalleries returns a paginated list of galleries.
func (db *DB) ListGalleries(ctx context.Context, f GalleryFilter) ([]models.Gallery, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := `SELECT id, source_id, provider, provider_gallery_id, title, url,
	                 thumbnail_url, local_thumbnail_path, created_at
	            FROM galleries WHERE 1=1`
	args := []any{}

	if f.SourceID != nil {
		query += " AND source_id = ?"
		args = append(args, *f.SourceID)
	}
	if f.Provider != nil {
		query += " AND provider = ?"
		args = append(args, *f.Provider)
	}
	if f.Search != nil {
		query += " AND title LIKE ?"
		args = append(args, "%"+*f.Search+"%")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	galleries := []models.Gallery{}
	if err := db.SelectContext(ctx, &galleries, query, args...); err != nil {
		return nil, fmt.Errorf("listing galleries: %w", err)
	}
	return galleries, nil
}

// GetGallery retrieves a single gallery by ID.
func (db *DB) GetGallery(ctx context.Context, id int64) (*models.Gallery, error) {
	var g models.Gallery
	err := db.GetContext(ctx, &g,
		`SELECT id, source_id, provider, provider_gallery_id, title, url,
		        thumbnail_url, local_thumbnail_path, created_at
		   FROM galleries WHERE id = ?`, id,
	)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting gallery %d: %w", id, err)
	}
	return &g, nil
}

// CreateGallery inserts a new gallery and populates the ID and CreatedAt.
func (db *DB) CreateGallery(ctx context.Context, g *models.Gallery) error {
	result, err := db.ExecContext(ctx,
		`INSERT INTO galleries (source_id, provider, provider_gallery_id, title, url, thumbnail_url, local_thumbnail_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.SourceID, g.Provider, g.ProviderGalleryID, g.Title, g.URL, g.ThumbnailURL, g.LocalThumbnailPath,
	)
	if err != nil {
		return fmt.Errorf("creating gallery: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting gallery id: %w", err)
	}
	g.ID = id
	g.CreatedAt = time.Now().UTC()
	return nil
}

// UpdateGallery updates mutable fields on an existing gallery.
func (db *DB) UpdateGallery(ctx context.Context, g *models.Gallery) error {
	_, err := db.ExecContext(ctx,
		`UPDATE galleries
		    SET source_id = ?, provider = ?, provider_gallery_id = ?,
		        title = ?, url = ?, thumbnail_url = ?, local_thumbnail_path = ?
		  WHERE id = ?`,
		g.SourceID, g.Provider, g.ProviderGalleryID,
		g.Title, g.URL, g.ThumbnailURL, g.LocalThumbnailPath, g.ID,
	)
	if err != nil {
		return fmt.Errorf("updating gallery %d: %w", g.ID, err)
	}
	return nil
}

// DeleteGallery removes a gallery by ID (cascades to images).
func (db *DB) DeleteGallery(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM galleries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting gallery %d: %w", id, err)
	}
	return nil
}

// SetGalleryThumbnail sets the local_thumbnail_path for a gallery, but only
// if it does not already have one (first-image-wins).
func (db *DB) SetGalleryThumbnail(ctx context.Context, galleryID int64, thumbPath string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE galleries SET local_thumbnail_path = ?
		  WHERE id = ? AND (local_thumbnail_path IS NULL OR local_thumbnail_path = '')`,
		thumbPath, galleryID,
	)
	if err != nil {
		return fmt.Errorf("setting gallery %d thumbnail: %w", galleryID, err)
	}
	return nil
}

// CountGalleries returns the total number of galleries.
func (db *DB) CountGalleries(ctx context.Context) (int64, error) {
	var count int64
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM galleries`); err != nil {
		return 0, fmt.Errorf("counting galleries: %w", err)
	}
	return count, nil
}

// ProviderCount holds a provider name and its gallery count.
type ProviderCount struct {
	Provider string `db:"provider" json:"provider"`
	Count    int64  `db:"count"    json:"count"`
}

// GalleryProviderBreakdown returns gallery counts grouped by provider.
func (db *DB) GalleryProviderBreakdown(ctx context.Context) ([]ProviderCount, error) {
	counts := []ProviderCount{}
	err := db.SelectContext(ctx, &counts,
		`SELECT COALESCE(provider, 'unknown') AS provider, COUNT(*) AS count
		   FROM galleries
		  GROUP BY provider
		  ORDER BY count DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("gallery provider breakdown: %w", err)
	}
	return counts, nil
}

// ListOrphanedImages returns images that are not linked to any gallery
// (gallery_id IS NULL or references a non-existent gallery).
func (db *DB) ListOrphanedImages(ctx context.Context) ([]models.Image, error) {
	images := []models.Image{}
	err := db.SelectContext(ctx, &images,
		`SELECT i.id, i.gallery_id, i.filename, i.original_url, i.width, i.height,
		        i.duration_seconds, i.file_hash, i.dominant_colors,
		        i.is_video, i.vr_mode, i.is_favorite, i.created_at
		   FROM images i
		   LEFT JOIN galleries g ON g.id = i.gallery_id
		  WHERE i.gallery_id IS NULL OR g.id IS NULL`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing orphaned images: %w", err)
	}
	return images, nil
}

// DeleteOrphanedImages removes images not linked to any gallery and returns
// the number of rows deleted.
func (db *DB) DeleteOrphanedImages(ctx context.Context) (int64, error) {
	result, err := db.ExecContext(ctx,
		`DELETE FROM images
		  WHERE gallery_id IS NULL
		     OR gallery_id NOT IN (SELECT id FROM galleries)`,
	)
	if err != nil {
		return 0, fmt.Errorf("deleting orphaned images: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}
	return n, nil
}

// ListGalleryPeople returns all people linked to a gallery.
func (db *DB) ListGalleryPeople(ctx context.Context, galleryID int64) ([]models.Person, error) {
	people := []models.Person{}
	err := db.SelectContext(ctx, &people,
		`SELECT p.id, p.name, p.aliases, p.birth_date, p.nationality, p.created_at
		   FROM people p
		   JOIN gallery_persons gp ON gp.person_id = p.id
		  WHERE gp.gallery_id = ?
		  ORDER BY p.name ASC`, galleryID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing gallery %d people: %w", galleryID, err)
	}
	return people, nil
}
