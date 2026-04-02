package ws

import (
	"sync"
	"sync/atomic"
	"time"
)

// QueueStats is the real-time status payload broadcast to WebSocket clients.
type QueueStats struct {
	Type      string       `json:"type"`      // always "queue_status"
	Queue     QueueCounts  `json:"queue"`     // global queue counters
	Active    []ActiveItem `json:"active"`    // currently processing items
	Timestamp int64        `json:"timestamp"` // unix millis
}

// QueueCounts holds aggregate counts by status.
type QueueCounts struct {
	Pending   int32 `json:"pending"`
	Active    int32 `json:"active"`
	Completed int32 `json:"completed"`
	Failed    int32 `json:"failed"`
}

// ActiveItem describes a single queue item being processed right now.
type ActiveItem struct {
	ID        int64  `json:"id"`
	URL       string `json:"url"`
	Type      string `json:"type"`       // image, video, gallery, crawl
	StartedAt int64  `json:"started_at"` // unix millis
}

// StatusTracker tracks queue processing state and broadcasts updates via the Hub.
// It is safe for concurrent use.
type StatusTracker struct {
	hub *Hub

	counts QueueCounts

	activeMu    sync.RWMutex
	activeItems []ActiveItem

	// throttle prevents flooding: at most one broadcast per interval.
	interval time.Duration
	lastSent atomic.Int64 // unix nanos of last broadcast
}

// NewStatusTracker creates a tracker bound to a hub.
func NewStatusTracker(hub *Hub) *StatusTracker {
	return &StatusTracker{
		hub:      hub,
		interval: 250 * time.Millisecond, // broadcast at most 4x/sec
	}
}

// ItemStarted records that a queue item began processing.
func (st *StatusTracker) ItemStarted(id int64, url, queueType string) {
	atomic.AddInt32(&st.counts.Active, 1)
	atomic.AddInt32(&st.counts.Pending, -1)

	st.activeMu.Lock()
	st.activeItems = append(st.activeItems, ActiveItem{
		ID:        id,
		URL:       url,
		Type:      queueType,
		StartedAt: time.Now().UnixMilli(),
	})
	st.activeMu.Unlock()

	st.maybeBroadcast()
}

// ItemCompleted records that a queue item finished successfully.
func (st *StatusTracker) ItemCompleted(id int64) {
	atomic.AddInt32(&st.counts.Active, -1)
	atomic.AddInt32(&st.counts.Completed, 1)
	st.removeActive(id)
	st.maybeBroadcast()
}

// ItemFailed records that a queue item failed.
func (st *StatusTracker) ItemFailed(id int64) {
	atomic.AddInt32(&st.counts.Active, -1)
	atomic.AddInt32(&st.counts.Failed, 1)
	st.removeActive(id)
	st.maybeBroadcast()
}

// SetPending sets the pending count (typically from a DB count on startup).
func (st *StatusTracker) SetPending(n int32) {
	atomic.StoreInt32(&st.counts.Pending, n)
	st.forceBroadcast()
}

// ItemEnqueued increments the pending count when a new item is added to the queue.
func (st *StatusTracker) ItemEnqueued() {
	atomic.AddInt32(&st.counts.Pending, 1)
	st.maybeBroadcast()
}

func (st *StatusTracker) removeActive(id int64) {
	st.activeMu.Lock()
	defer st.activeMu.Unlock()
	for i, item := range st.activeItems {
		if item.ID == id {
			st.activeItems = append(st.activeItems[:i], st.activeItems[i+1:]...)
			return
		}
	}
}

// Snapshot returns the current status as a QueueStats struct.
func (st *StatusTracker) Snapshot() QueueStats {
	st.activeMu.RLock()
	active := make([]ActiveItem, len(st.activeItems))
	copy(active, st.activeItems)
	st.activeMu.RUnlock()

	return QueueStats{
		Type: "queue_status",
		Queue: QueueCounts{
			Pending:   atomic.LoadInt32(&st.counts.Pending),
			Active:    atomic.LoadInt32(&st.counts.Active),
			Completed: atomic.LoadInt32(&st.counts.Completed),
			Failed:    atomic.LoadInt32(&st.counts.Failed),
		},
		Active:    active,
		Timestamp: time.Now().UnixMilli(),
	}
}

// maybeBroadcast sends a status update if enough time has elapsed since the
// last broadcast.
func (st *StatusTracker) maybeBroadcast() {
	now := time.Now().UnixNano()
	last := st.lastSent.Load()
	if time.Duration(now-last) < st.interval {
		return
	}
	if st.lastSent.CompareAndSwap(last, now) {
		st.hub.Broadcast(st.Snapshot())
	}
}

// forceBroadcast always sends, ignoring the throttle.
func (st *StatusTracker) forceBroadcast() {
	st.lastSent.Store(time.Now().UnixNano())
	st.hub.Broadcast(st.Snapshot())
}
