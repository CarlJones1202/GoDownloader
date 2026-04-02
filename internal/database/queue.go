// Package database — download queue queries.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/carlj/godownload/internal/models"
)

// QueueFilter holds optional filter parameters for ListQueue.
type QueueFilter struct {
	Status *string
	Type   *string
	Limit  int
	Offset int
}

// ListQueue returns a paginated view of the download queue.
func (db *DB) ListQueue(ctx context.Context, f QueueFilter) ([]models.DownloadQueue, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := `SELECT dq.id, dq.type, dq.url, dq.target_id, dq.status,
	                 dq.retry_count, dq.error_message, dq.created_at,
	                 g.title AS gallery_title,
	                 s.id AS source_id,
	                 s.name AS source_name
	            FROM download_queue dq
	       LEFT JOIN galleries g ON dq.target_id = g.id AND dq.type IN ('image', 'video')
	       LEFT JOIN sources   s ON g.source_id = s.id
	           WHERE 1=1`
	args := []any{}

	if f.Status != nil {
		query += " AND dq.status = ?"
		args = append(args, *f.Status)
	}
	if f.Type != nil {
		query += " AND dq.type = ?"
		args = append(args, *f.Type)
	}

	query += " ORDER BY dq.created_at ASC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	items := []models.DownloadQueue{}
	if err := db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, fmt.Errorf("listing queue: %w", err)
	}
	return items, nil
}

// GetQueueItem retrieves a single queue item by ID.
func (db *DB) GetQueueItem(ctx context.Context, id int64) (*models.DownloadQueue, error) {
	var item models.DownloadQueue
	err := db.GetContext(ctx, &item,
		`SELECT id, type, url, target_id, status, retry_count, error_message, created_at
		   FROM download_queue WHERE id = ?`, id,
	)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting queue item %d: %w", id, err)
	}
	return &item, nil
}

// EnqueueItem adds a new item to the download queue.
func (db *DB) EnqueueItem(ctx context.Context, item *models.DownloadQueue) error {
	result, err := db.ExecContext(ctx,
		`INSERT INTO download_queue (type, url, target_id, status)
		 VALUES (?, ?, ?, ?)`,
		item.Type, item.URL, item.TargetID, models.QueueStatusPending,
	)
	if err != nil {
		return fmt.Errorf("enqueueing item: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting queue item id: %w", err)
	}
	item.ID = id
	item.Status = string(models.QueueStatusPending)
	item.CreatedAt = time.Now().UTC()
	return nil
}

// UpdateQueueStatus updates the status (and optionally error message) for a queue item.
func (db *DB) UpdateQueueStatus(ctx context.Context, id int64, status models.QueueStatus, errMsg *string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE download_queue SET status = ?, error_message = ? WHERE id = ?`,
		status, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("updating queue item %d status: %w", id, err)
	}
	return nil
}

// IncrementRetry increments retry_count and sets status back to pending for a queue item.
func (db *DB) IncrementRetry(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE download_queue
		    SET retry_count = retry_count + 1, status = ?
		  WHERE id = ?`,
		models.QueueStatusPending, id,
	)
	if err != nil {
		return fmt.Errorf("incrementing retry for queue item %d: %w", id, err)
	}
	return nil
}

// ResetActiveToPending resets all "active" items to "pending". Used on startup
// to recover from crashes.
func (db *DB) ResetActiveToPending(ctx context.Context) (int64, error) {
	result, err := db.ExecContext(ctx,
		`UPDATE download_queue SET status = ? WHERE status = ?`,
		models.QueueStatusPending, models.QueueStatusActive,
	)
	if err != nil {
		return 0, fmt.Errorf("resetting active to pending: %w", err)
	}
	return result.RowsAffected()
}

// DeleteQueueItem removes a queue item by ID.
func (db *DB) DeleteQueueItem(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM download_queue WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting queue item %d: %w", id, err)
	}
	return nil
}

// QueueStats holds aggregate queue counts by status.
type QueueStats struct {
	Pending   int64 `db:"pending"   json:"pending"`
	Active    int64 `db:"active"    json:"active"`
	Completed int64 `db:"completed" json:"completed"`
	Failed    int64 `db:"failed"    json:"failed"`
	Paused    int64 `db:"paused"    json:"paused"`
}

// GetQueueStats returns aggregate queue counts by status.
func (db *DB) GetQueueStats(ctx context.Context) (*QueueStats, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT status, COUNT(*) as cnt FROM download_queue GROUP BY status`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying queue stats: %w", err)
	}
	defer rows.Close()

	stats := &QueueStats{}
	for rows.Next() {
		var status string
		var cnt int64
		if err := rows.Scan(&status, &cnt); err != nil {
			return nil, fmt.Errorf("scanning queue stat: %w", err)
		}
		switch models.QueueStatus(status) {
		case models.QueueStatusPending:
			stats.Pending = cnt
		case models.QueueStatusActive:
			stats.Active = cnt
		case models.QueueStatusCompleted:
			stats.Completed = cnt
		case models.QueueStatusFailed:
			stats.Failed = cnt
		case models.QueueStatusPaused:
			stats.Paused = cnt
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating queue stats: %w", err)
	}

	return stats, nil
}

// DownloadStats holds temporal download statistics.
type DownloadStats struct {
	CompletedToday int64 `db:"completed_today" json:"completed_today"`
	CompletedWeek  int64 `db:"completed_week"  json:"completed_week"`
	FailedToday    int64 `db:"failed_today"    json:"failed_today"`
	FailedWeek     int64 `db:"failed_week"     json:"failed_week"`
}

// GetDownloadStats returns temporal download statistics (completed/failed today and this week).
func (db *DB) GetDownloadStats(ctx context.Context) (*DownloadStats, error) {
	var stats DownloadStats
	err := db.GetContext(ctx, &stats, `
		SELECT
		  COALESCE(SUM(CASE WHEN status = 'completed' AND created_at >= date('now') THEN 1 ELSE 0 END), 0)   AS completed_today,
		  COALESCE(SUM(CASE WHEN status = 'completed' AND created_at >= date('now', '-7 days') THEN 1 ELSE 0 END), 0) AS completed_week,
		  COALESCE(SUM(CASE WHEN status = 'failed'    AND created_at >= date('now') THEN 1 ELSE 0 END), 0)   AS failed_today,
		  COALESCE(SUM(CASE WHEN status = 'failed'    AND created_at >= date('now', '-7 days') THEN 1 ELSE 0 END), 0) AS failed_week
		FROM download_queue`,
	)
	if err != nil {
		return nil, fmt.Errorf("getting download stats: %w", err)
	}
	return &stats, nil
}

// ClearQueue deletes queue items, optionally filtered by status.
// Returns the number of deleted rows.
func (db *DB) ClearQueue(ctx context.Context, status *string) (int64, error) {
	query := `DELETE FROM download_queue`
	args := []any{}

	if status != nil {
		query += " WHERE status = ?"
		args = append(args, *status)
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("clearing queue: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}
	return n, nil
}

// NextPendingItems fetches up to n pending queue items and marks them as active.
func (db *DB) NextPendingItems(ctx context.Context, n int) ([]models.DownloadQueue, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	items := []models.DownloadQueue{}
	err = tx.SelectContext(ctx, &items,
		`SELECT id, type, url, target_id, status, retry_count, error_message, created_at
		   FROM download_queue
		  WHERE status = ?
		  ORDER BY created_at ASC
		  LIMIT ?`,
		models.QueueStatusPending, n,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching pending queue items: %w", err)
	}

	for _, item := range items {
		if _, err := tx.ExecContext(ctx,
			`UPDATE download_queue SET status = ? WHERE id = ?`,
			models.QueueStatusActive, item.ID,
		); err != nil {
			return nil, fmt.Errorf("marking item %d active: %w", item.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return items, nil
}
