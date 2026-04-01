// Package database — source queries.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/carlj/godownload/internal/models"
)

// ListSources returns all sources ordered by priority desc, name asc.
func (db *DB) ListSources(ctx context.Context) ([]models.Source, error) {
	sources := []models.Source{}
	err := db.SelectContext(ctx, &sources,
		`SELECT id, url, name, enabled, priority, last_crawled_at, created_at
		   FROM sources
		  ORDER BY priority DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	return sources, nil
}

// GetSource retrieves a single source by ID.
func (db *DB) GetSource(ctx context.Context, id int64) (*models.Source, error) {
	var s models.Source
	err := db.GetContext(ctx, &s,
		`SELECT id, url, name, enabled, priority, last_crawled_at, created_at
		   FROM sources WHERE id = ?`, id,
	)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting source %d: %w", id, err)
	}
	return &s, nil
}

// CreateSource inserts a new source and returns it with the assigned ID.
func (db *DB) CreateSource(ctx context.Context, s *models.Source) error {
	result, err := db.ExecContext(ctx,
		`INSERT INTO sources (url, name, enabled, priority)
		 VALUES (?, ?, ?, ?)`,
		s.URL, s.Name, s.Enabled, s.Priority,
	)
	if err != nil {
		return fmt.Errorf("creating source: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting source id: %w", err)
	}
	s.ID = id
	s.CreatedAt = time.Now().UTC()
	return nil
}

// UpdateSource updates mutable fields on an existing source.
func (db *DB) UpdateSource(ctx context.Context, s *models.Source) error {
	_, err := db.ExecContext(ctx,
		`UPDATE sources SET url = ?, name = ?, enabled = ?, priority = ?
		  WHERE id = ?`,
		s.URL, s.Name, s.Enabled, s.Priority, s.ID,
	)
	if err != nil {
		return fmt.Errorf("updating source %d: %w", s.ID, err)
	}
	return nil
}

// TouchSourceCrawledAt sets last_crawled_at to now for a source.
func (db *DB) TouchSourceCrawledAt(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE sources SET last_crawled_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("touching source %d crawled_at: %w", id, err)
	}
	return nil
}

// DeleteSource removes a source by ID.
func (db *DB) DeleteSource(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting source %d: %w", id, err)
	}
	return nil
}

// CountSources returns the total number of sources.
func (db *DB) CountSources(ctx context.Context) (int64, error) {
	var count int64
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM sources`); err != nil {
		return 0, fmt.Errorf("counting sources: %w", err)
	}
	return count, nil
}
