// Package queue implements the download queue worker that processes
// pending items from the download_queue table with configurable concurrency,
// retry logic, and graceful shutdown.
package queue

import (
	"context"
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

// Manager polls the database for pending queue items and dispatches them
// to registered processors using a bounded worker pool.
type Manager struct {
	db         *database.DB
	processors map[string]Processor
	workers    int
	maxRetries int

	paused atomic.Bool
	stopCh chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

// New creates a Manager with the given number of workers and max retries.
func New(db *database.DB, workers, maxRetries int) *Manager {
	return &Manager{
		db:         db,
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

// Start begins the polling loop. It is non-blocking.
func (m *Manager) Start() {
	m.wg.Add(1)
	go m.loop()
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
			items, err := m.db.NextPendingItems(ctx, defaultBatchSize)
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
		_ = m.db.UpdateQueueStatus(
			context.Background(),
			item.ID,
			models.QueueStatusFailed,
			&msg,
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := proc.Process(ctx, item)
	if err == nil {
		slog.Info("queue: item completed", "id", item.ID, "type", item.Type)
		_ = m.db.UpdateQueueStatus(ctx, item.ID, models.QueueStatusCompleted, nil)
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
		_ = m.db.UpdateQueueStatus(ctx, item.ID, models.QueueStatusFailed, &msg)
		return
	}

	_ = m.db.IncrementRetry(ctx, item.ID)
}
