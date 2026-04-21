// Package queue implements the download queue worker that processes
// pending items from the download_queue table with configurable concurrency,
// retry logic, and graceful shutdown.
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

const (
	defaultPollInterval  = 2 * time.Second
	defaultBatchSize     = 10
	defaultProviderLimit = 3 // max concurrent downloads per image host
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

// ActiveDownload holds information about an in-flight queue item.
type ActiveDownload struct {
	ID        int64
	URL       string
	Type      string
	Provider  string
	StartedAt time.Time
}

// Manager polls the database for pending queue items and dispatches them
// to registered processors using a bounded worker pool.
type Manager struct {
	db            *database.DB
	dbWriter      DBWriter
	processors    map[string]Processor
	workers       int
	providerLimit int // max concurrent downloads per provider
	maxRetries    int

	// per-provider semaphores (lazy-initialised)
	providerMu   sync.Mutex
	providerSems map[string]chan struct{}

	// active downloads for admin visibility
	activeMu      sync.RWMutex
	activeDownloads []ActiveDownload

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
		db:            db,
		dbWriter:      dbWriter,
		processors:    map[string]Processor{},
		workers:       workers,
		providerLimit: defaultProviderLimit,
		maxRetries:    maxRetries,
		stopCh:        make(chan struct{}),
		providerSems:  make(map[string]chan struct{}),
	}
}

// SetProviderLimit overrides the per-provider concurrency cap (default: 3).
func (m *Manager) SetProviderLimit(n int) {
	if n >= 1 {
		m.providerLimit = n
	}
}

// providerSem returns (and lazily creates) the semaphore for a given provider.
func (m *Manager) providerSem(provider string) chan struct{} {
	m.providerMu.Lock()
	defer m.providerMu.Unlock()
	if ch, ok := m.providerSems[provider]; ok {
		return ch
	}
	ch := make(chan struct{}, m.providerLimit)
	m.providerSems[provider] = ch
	return ch
}

// providerFor extracts a normalised provider key from a queue item URL.
// For image/video/gallery types it uses the URL hostname; crawl items use
// the literal string "crawl" so they share a single semaphore.
func providerFor(item *models.DownloadQueue) string {
	if item.Type == string(models.QueueTypeCrawl) {
		return "crawl"
	}
	// Strip pipe-separated thumbnail suffix added by the crawler.
	raw := item.URL
	if idx := strings.Index(raw, "|"); idx >= 0 {
		raw = raw[:idx]
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	host := strings.ToLower(u.Hostname())
	// Normalise: strip leading "www."
	host = strings.TrimPrefix(host, "www.")
	return host
}

// ActiveDownloads returns a snapshot of currently-processing queue items.
func (m *Manager) ActiveDownloads() []ActiveDownload {
	m.activeMu.RLock()
	defer m.activeMu.RUnlock()
	out := make([]ActiveDownload, len(m.activeDownloads))
	copy(out, m.activeDownloads)
	return out
}

// trackStarted records that an item began processing.
func (m *Manager) trackStarted(item *models.DownloadQueue, provider string) {
	m.activeMu.Lock()
	m.activeDownloads = append(m.activeDownloads, ActiveDownload{
		ID:        item.ID,
		URL:       item.URL,
		Type:      item.Type,
		Provider:  provider,
		StartedAt: time.Now(),
	})
	m.activeMu.Unlock()
}

// trackFinished removes an item from the active list.
func (m *Manager) trackFinished(id int64) {
	m.activeMu.Lock()
	for i, ad := range m.activeDownloads {
		if ad.ID == id {
			m.activeDownloads = append(m.activeDownloads[:i], m.activeDownloads[i+1:]...)
			break
		}
	}
	m.activeMu.Unlock()
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

	// Global semaphore to bound total concurrent workers.
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
				provider := providerFor(&item)
				provSem := m.providerSem(provider)

				// Try to acquire the per-provider slot (non-blocking).
				// If the provider is already at capacity, skip this item for
				// now — it will be retried on the next poll tick.
				select {
				case provSem <- struct{}{}:
					// acquired provider slot — continue to acquire global slot
				default:
					// Provider at capacity: reset item to pending so it is
					// picked up again next poll.
					slog.Debug("queue: provider at capacity, deferring item",
						"id", item.ID, "provider", provider)
					_ = m.dbWriter.UpdateQueueStatus(
						context.Background(), item.ID, models.QueueStatusPending, nil)
					continue
				}

				// Acquire global slot (blocking — other items may release it).
				sem <- struct{}{}
				m.wg.Add(1)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("queue: worker panicked", "id", item.ID, "type", item.Type, "panic", r)
							msg := fmt.Sprintf("panic: %v", r)
							_ = m.dbWriter.UpdateQueueStatus(context.Background(), item.ID, models.QueueStatusFailed, &msg)
							m.trackFinished(item.ID)
						}
						<-provSem // release provider slot
						<-sem     // release global slot
						m.wg.Done()
					}()
					m.trackStarted(&item, provider)
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
		m.trackFinished(item.ID)
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
		m.trackFinished(item.ID)
		if m.statusTracker != nil {
			m.statusTracker.ItemFailed(item.ID)
		}
		return
	}

	_ = m.dbWriter.IncrementRetry(statusCtx, item.ID)
}
