// Package queue implements the download queue worker that processes
// pending items from the download_queue table with configurable concurrency,
// retry logic, and graceful shutdown.
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

const (
	defaultPollInterval = 2 * time.Second
	defaultBatchSize    = 10
)

// Processor is the interface that queue item handlers must implement.
// Each QueueType should have its own Processor registered via RegisterProcessor.
type Processor interface {
	Process(ctx context.Context, item *models.DownloadQueue) error
}

// ProcessorFunc is a convenience adapter that allows use of plain functions
// as Processors.
type ProcessorFunc func(ctx context.Context, item *models.DownloadQueue) error

func (f ProcessorFunc) Process(ctx context.Context, item *models.DownloadQueue) error {
	return f(ctx, item)
}

// DBWriter interface for database operations that should be serialized.
type DBWriter interface {
	ResetActiveToPending(ctx context.Context) (int64, error)
	UpdateQueueStatus(ctx context.Context, id int64, status models.QueueStatus, errMsg *string) error
	IncrementRetry(ctx context.Context, id int64) error
}

// Manager polls the database for pending queue items and dispatches them
// to registered processors using a bounded worker pool.
type Manager struct {
	db         *database.DB
	dbWriter   DBWriter
	processors map[string]Processor
	workers    int
	maxRetries int

	paused atomic.Bool
	stopCh chan struct{}
	wg     sync.WaitGroup
	once   sync.Once

	// StatusTracker is an optional WebSocket status broadcaster.
	// If non-nil, queue events are reported to connected clients.
	statusTracker statusReporter
}

// statusReporter is the subset of ws.StatusTracker we need, defined here to
// avoid importing the ws package (keeps the dependency one-directional).
type statusReporter interface {
	ItemStarted(id int64, url, queueType string)
	ItemCompleted(id int64)
	ItemFailed(id int64)
}

// New creates a Manager with the given number of workers and max retries.
func New(db *database.DB, dbWriter DBWriter, workers, maxRetries int) *Manager {
	return &Manager{
		db:         db,
		dbWriter:   dbWriter,
		processors: map[string]Processor{},
		workers:    workers,
		maxRetries: maxRetries,
		stopCh:     make(chan struct{}),
	}
}

// RegisterProcessor associates a queue type with its processor.
func (m *Manager) RegisterProcessor(queueType models.QueueType, p Processor) {
	m.processors[string(queueType)] = p
}

// SetStatusTracker attaches a WebSocket status reporter to the manager.
func (m *Manager) SetStatusTracker(st statusReporter) {
	m.statusTracker = st
}

// Start begins the polling loop. It is non-blocking.
func (m *Manager) Start() {
	m.recoverStuckItems()
	m.wg.Add(1)
	go m.loop()
}

// recoverStuckItems resets any "active" items to "pending" on startup.
// This handles cases where the server crashed or was killed mid-download.
func (m *Manager) recoverStuckItems() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := m.dbWriter.ResetActiveToPending(ctx)
	if err != nil {
		slog.Error("queue: recovery: failed to reset stuck items", "error", err)
		return
	}
	if rows > 0 {
		slog.Info("queue: recovery: reset stuck items", "count", rows)
	}
}

// Stop signals the manager to stop accepting new work and waits for
// in-flight jobs to complete.
func (m *Manager) Stop() {
	m.once.Do(func() {
		close(m.stopCh)
		m.wg.Wait()
	})
}

// Pause temporarily halts processing without stopping the manager.
func (m *Manager) Pause() { m.paused.Store(true) }

// Resume resumes processing after a Pause.
func (m *Manager) Resume() { m.paused.Store(false) }

// IsPaused reports whether the manager is currently paused.
func (m *Manager) IsPaused() bool { return m.paused.Load() }

// loop is the main polling goroutine.
func (m *Manager) loop() {
	defer m.wg.Done()

	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	// Semaphore to bound concurrent workers.
	sem := make(chan struct{}, m.workers)

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.paused.Load() {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			items, err := m.db.NextPendingItems(ctx, m.workers)
			cancel()

			if err != nil {
				slog.Error("queue: fetching pending items", "error", err)
				continue
			}

			for i := range items {
				item := items[i]

				// Acquire slot.
				sem <- struct{}{}
				m.wg.Add(1)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("queue: worker panicked", "id", item.ID, "type", item.Type, "panic", r)
							msg := fmt.Sprintf("panic: %v", r)
							_ = m.dbWriter.UpdateQueueStatus(context.Background(), item.ID, models.QueueStatusFailed, &msg)
						}
						<-sem // release slot
						m.wg.Done()
					}()
					m.dispatch(&item)
				}()
			}
		}
	}
}

// dispatch processes a single queue item, handling retries and status updates.
func (m *Manager) dispatch(item *models.DownloadQueue) {
	proc, ok := m.processors[item.Type]
	if !ok {
		slog.Warn("queue: no processor registered", "type", item.Type, "id", item.ID)
		msg := "no processor registered for type: " + item.Type
		_ = m.dbWriter.UpdateQueueStatus(
			context.Background(),
			item.ID,
			models.QueueStatusFailed,
			&msg,
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Notify status tracker that processing started.
	if m.statusTracker != nil {
		m.statusTracker.ItemStarted(item.ID, item.URL, item.Type)
	}

	err := proc.Process(ctx, item)

	// Use a fresh context for status updates — the processing context may
	// have expired or been cancelled, and we must always record the outcome.
	statusCtx, statusCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer statusCancel()

	if err == nil {
		slog.Info("queue: item completed", "id", item.ID, "type", item.Type)
		_ = m.dbWriter.UpdateQueueStatus(statusCtx, item.ID, models.QueueStatusCompleted, nil)
		if m.statusTracker != nil {
			m.statusTracker.ItemCompleted(item.ID)
		}
		return
	}

	slog.Error(
		"queue: item failed",
		"id", item.ID,
		"type", item.Type,
		"error", err,
		"retry_count", item.RetryCount,
	)

	if item.RetryCount >= m.maxRetries {
		msg := err.Error()
		_ = m.dbWriter.UpdateQueueStatus(statusCtx, item.ID, models.QueueStatusFailed, &msg)
		if m.statusTracker != nil {
			m.statusTracker.ItemFailed(item.ID)
		}
		return
	}

	_ = m.dbWriter.IncrementRetry(statusCtx, item.ID)
}
