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
// to registered processors using a bounded worker pool with per-provider limits.
type Manager struct {
	db            *database.DB
	dbWriter      DBWriter
	processors    map[string]Processor
	workers       int
	providerLimit int // max concurrent downloads per provider
	providerPool  int // items fetched from DB per provider per poll tick
	maxRetries    int

	// active downloads — used for admin visibility and balanced scheduling
	activeMu        sync.RWMutex
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
		providerPool:  10, // fetch 10 pending items per provider per poll tick
		maxRetries:    maxRetries,
		stopCh:        make(chan struct{}),
	}
}

// SetProviderLimit overrides the per-provider concurrency cap (default: 3).
func (m *Manager) SetProviderLimit(n int) {
	if n >= 1 {
		m.providerLimit = n
	}
}

// SetProviderPool overrides how many items are fetched per provider per poll (default: 10).
func (m *Manager) SetProviderPool(n int) {
	if n >= 1 {
		m.providerPool = n
	}
}

// activeCount returns the number of currently-running downloads.
func (m *Manager) activeCount() int {
	m.activeMu.RLock()
	defer m.activeMu.RUnlock()
	return len(m.activeDownloads)
}

// activeByProvider returns a snapshot of provider → currently-running count.
func (m *Manager) activeByProvider() map[string]int {
	m.activeMu.RLock()
	defer m.activeMu.RUnlock()
	counts := make(map[string]int, len(m.activeDownloads))
	for _, ad := range m.activeDownloads {
		counts[ad.Provider]++
	}
	return counts
}

// selectBalanced picks items from pool respecting per-provider concurrency limits.
// It uses a round-robin strategy: one item is picked from each provider per pass,
// cycling until maxTotal items are selected or all providers are exhausted or at capacity.
// This ensures every provider gets slots even when one provider dominates the pool
// by creation-time ordering.
func (m *Manager) selectBalanced(pool []models.DownloadQueue, maxTotal int) []models.DownloadQueue {
	// Group pool items by provider, preserving intra-provider creation-time order.
	type providerQueue struct {
		name  string
		items []models.DownloadQueue
	}
	byProvider := map[string]*providerQueue{}
	var order []string // stable insertion order for round-robin

	for _, item := range pool {
		p := providerFor(&item)
		if _, ok := byProvider[p]; !ok {
			byProvider[p] = &providerQueue{name: p}
			order = append(order, p)
		}
		byProvider[p].items = append(byProvider[p].items, item)
	}

	// Per-provider remaining slots = providerLimit - currently active.
	active := m.activeByProvider()
	slots := make(map[string]int, len(order))
	for _, p := range order {
		if avail := m.providerLimit - active[p]; avail > 0 {
			slots[p] = avail
		}
	}

	// Round-robin: each pass picks at most 1 item from each provider.
	// We continue until maxTotal is reached or no progress is made.
	var selected []models.DownloadQueue
	idx := make(map[string]int, len(order)) // next-item index per provider

	for len(selected) < maxTotal {
		progress := false
		for _, p := range order {
			if len(selected) >= maxTotal {
				break
			}
			if slots[p] <= 0 {
				continue
			}
			i := idx[p]
			if i >= len(byProvider[p].items) {
				continue
			}
			selected = append(selected, byProvider[p].items[i])
			idx[p]++
			slots[p]--
			progress = true
		}
		if !progress {
			break // all providers exhausted or at capacity
		}
	}
	return selected
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

	// Global semaphore caps the total number of concurrent goroutines.
	sem := make(chan struct{}, m.workers)

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.paused.Load() {
				continue
			}

			// How many more workers can we start right now?
			available := m.workers - m.activeCount()
			if available <= 0 {
				continue
			}

			// Fetch up to providerPool items PER PROVIDER from the DB.
			// The window-function query partitions by hostname so every provider
			// gets equal representation regardless of creation-time ordering.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			pool, err := m.db.PeekPendingItems(ctx, m.providerPool)
			cancel()

			if err != nil {
				slog.Error("queue: fetching pending items", "error", err)
				continue
			}
			if len(pool) == 0 {
				continue
			}

			// Pick a balanced subset: up to providerLimit concurrent per provider.
			selected := m.selectBalanced(pool, available)
			if len(selected) == 0 {
				continue
			}

			// Atomically mark the selected items active in the DB.
			ids := make([]int64, len(selected))
			for i, s := range selected {
				ids[i] = s.ID
			}
			markCtx, markCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := m.db.MarkItemsActive(markCtx, ids); err != nil {
				slog.Error("queue: marking items active", "error", err)
				markCancel()
				continue
			}
			markCancel()

			slog.Debug("queue: dispatching balanced batch",
				"selected", len(selected),
				"pool_size", len(pool),
				"available_slots", available,
			)

			// Spawn a goroutine for each selected item.
			for i := range selected {
				item := selected[i]
				provider := providerFor(&item)

				// Track before spawning so activeByProvider is accurate for any
				// subsequent selectBalanced call within the same tick.
				m.trackStarted(&item, provider)

				sem <- struct{}{} // acquire global slot
				m.wg.Add(1)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("queue: worker panicked", "id", item.ID, "type", item.Type, "panic", r)
							msg := fmt.Sprintf("panic: %v", r)
							_ = m.dbWriter.UpdateQueueStatus(context.Background(), item.ID, models.QueueStatusFailed, &msg)
							m.trackFinished(item.ID)
						}
						<-sem
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
		m.trackFinished(item.ID)
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

	// Retry: increment counter, reset to pending. Item leaves the active list.
	_ = m.dbWriter.IncrementRetry(statusCtx, item.ID)
	m.trackFinished(item.ID)
}
