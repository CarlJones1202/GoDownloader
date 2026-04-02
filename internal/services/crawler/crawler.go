// Package crawler implements source crawling with a priority queue and
// configurable concurrency. Each crawl job is dispatched to a worker pool;
// workers enqueue discovered image/gallery downloads into the download_queue table.
package crawler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

// ImageLink holds a discovered image link from a forum post.
// PageURL is the <a href> (the image host page).
// ThumbURL is the <img src> (the thumbnail URL), which some rippers
// use to derive the full-size image via URL transformation.
type ImageLink struct {
	PageURL  string
	ThumbURL string
}

// SourceParser extracts gallery and image information from a crawled source page.
// Each forum/site type has its own implementation.
type SourceParser interface {
	// Hosts returns URL host patterns this parser handles,
	// e.g. []string{"vipergirls.to", "www.vipergirls.to"}.
	Hosts() []string
	// Parse receives the page HTML, page URL, and optional post ID filter.
	// If postID is non-empty, only that specific post is processed.
	// If postID is empty, only the first post is processed.
	// Returns discovered image links grouped by gallery title.
	Parse(ctx context.Context, body, pageURL, postID string) (map[string][]ImageLink, error)
}

// job represents a single crawl request.
type job struct {
	source   *models.Source
	fullSync bool // true = re-crawl all galleries, false = incremental
}

// Crawler manages a pool of workers that crawl source URLs.
type Crawler struct {
	db      *database.DB
	cfg     config.CrawlerConfig
	client  *http.Client
	parsers map[string]SourceParser

	jobs   chan job
	wg     sync.WaitGroup
	stopCh chan struct{}
	once   sync.Once
}

// New creates a Crawler and starts its worker pool.
func New(db *database.DB, cfg config.CrawlerConfig) *Crawler {
	c := &Crawler{
		db:  db,
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: cfg.RequestTimeout,
			},
		},
		parsers: make(map[string]SourceParser),
		jobs:    make(chan job, cfg.Workers*4),
		stopCh:  make(chan struct{}),
	}
	c.start()
	c.startScheduler()
	return c
}

// RegisterParser adds a SourceParser for all of its declared hosts.
func (c *Crawler) RegisterParser(p SourceParser) {
	for _, host := range p.Hosts() {
		c.parsers[strings.ToLower(host)] = p
	}
}

// parserFor returns the SourceParser for the given URL, or nil.
func (c *Crawler) parserFor(rawURL string) SourceParser {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if p, ok := c.parsers[host]; ok {
		return p
	}
	bare := strings.TrimPrefix(host, "www.")
	if p, ok := c.parsers[bare]; ok {
		return p
	}
	return nil
}

// start spawns the worker goroutines.
func (c *Crawler) start() {
	for range c.cfg.Workers {
		c.wg.Add(1)
		go c.worker()
	}
}

// Stop drains the job queue and waits for all workers to finish.
// It is safe to call Stop multiple times.
func (c *Crawler) Stop() {
	c.once.Do(func() {
		close(c.stopCh)
		c.wg.Wait()
	})
}

// startScheduler launches a background goroutine that periodically checks
// enabled sources and enqueues any that are stale (last_crawled_at older
// than cfg.CrawlInterval or NULL). If CrawlInterval is zero the scheduler
// is disabled.
func (c *Crawler) startScheduler() {
	interval := c.cfg.CrawlInterval
	if interval <= 0 {
		slog.Info("crawler: scheduler disabled (crawl_interval <= 0)")
		return
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		// Run an initial check shortly after startup.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-c.stopCh:
				return
			case <-timer.C:
				c.scheduleStale(interval)
				timer.Reset(interval)
			}
		}
	}()
}

// scheduleStale queries all enabled sources and enqueues those whose
// last_crawled_at is older than the crawl interval (or NULL).
func (c *Crawler) scheduleStale(interval time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sources, err := c.db.ListSources(ctx)
	if err != nil {
		slog.Error("crawler: scheduler listing sources", "error", err)
		return
	}

	cutoff := time.Now().Add(-interval)
	enqueued := 0

	for i := range sources {
		s := &sources[i]
		if !s.Enabled {
			continue
		}
		if s.LastCrawledAt != nil && s.LastCrawledAt.After(cutoff) {
			continue
		}
		c.enqueue(job{source: s, fullSync: false})
		enqueued++
	}

	if enqueued > 0 {
		slog.Info("crawler: scheduler enqueued stale sources",
			"count", enqueued,
			"interval", interval,
		)
	}
}

// EnqueueSource adds an incremental crawl job for the given source.
func (c *Crawler) EnqueueSource(src *models.Source) {
	c.enqueue(job{source: src, fullSync: false})
}

// EnqueueSourceFull adds a full re-crawl job for the given source.
func (c *Crawler) EnqueueSourceFull(src *models.Source) {
	c.enqueue(job{source: src, fullSync: true})
}

func (c *Crawler) enqueue(j job) {
	select {
	case c.jobs <- j:
	case <-c.stopCh:
	}
}

// worker pulls jobs from the channel and processes them until stopped.
func (c *Crawler) worker() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case j, ok := <-c.jobs:
			if !ok {
				return
			}
			c.process(j)
		}
	}
}

// process executes a single crawl job. It fetches the source page,
// parses it with the appropriate SourceParser, creates gallery records,
// and enqueues image download tasks.
func (c *Crawler) process(j job) {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.RequestTimeout*10)
	defer cancel()

	src := j.source

	slog.Info("crawling source",
		"source_id", src.ID,
		"url", src.URL,
		"full_sync", j.fullSync,
	)

	parser := c.parserFor(src.URL)
	if parser == nil {
		// No parser registered — fall back to the Phase 1 stub behaviour:
		// enqueue a crawl item and mark crawled_at.
		slog.Warn("crawler: no parser for source host, stub crawl only",
			"source_id", src.ID,
			"url", src.URL,
		)
		c.stubCrawl(ctx, j)
		return
	}

	// Extract post ID filter from URL fragment (#postXXX) or query param (?p=XXX).
	// If empty, the parser will process only the first post.
	postIDFilter := extractPostID(src.URL)

	// Fetch the page.
	body, err := c.fetchPage(ctx, src.URL)
	if err != nil {
		slog.Error("crawler: fetching source page",
			"error", err,
			"source_id", src.ID,
		)
		return
	}

	// Parse galleries from the page HTML with optional post filter.
	galleries, err := parser.Parse(ctx, body, src.URL, postIDFilter)
	if err != nil {
		slog.Error("crawler: parsing source page",
			"error", err,
			"source_id", src.ID,
		)
		return
	}

	// Create galleries and enqueue image downloads.
	totalImages := 0
	for title, imageLinks := range galleries {
		// Gallery title is ONLY the source name - never from page content.
		galleryTitle := src.Name
		if galleryTitle == "" {
			galleryTitle = title
		}

		galleryID, err := c.ensureGallery(ctx, src.ID, galleryTitle, src.URL)
		if err != nil {
			slog.Error("crawler: creating gallery",
				"error", err,
				"title", title,
			)
			continue
		}

		for _, link := range imageLinks {
			// Store the page URL as the primary URL, and encode the thumbnail
			// URL in a pipe-separated format so the queue processor can pass
			// it to ThumbnailRipper-aware rippers.
			queueURL := link.PageURL
			if link.ThumbURL != "" {
				queueURL = link.PageURL + "|" + link.ThumbURL
			}

			item := &models.DownloadQueue{
				Type:     string(models.QueueTypeImage),
				URL:      queueURL,
				TargetID: &galleryID,
			}
			if err := c.db.EnqueueItem(ctx, item); err != nil {
				slog.Error("crawler: enqueueing image",
					"error", err,
					"url", link.PageURL,
				)
				continue
			}
			totalImages++
		}
	}

	slog.Info("crawler: source crawl complete",
		"source_id", src.ID,
		"galleries", len(galleries),
		"images_enqueued", totalImages,
	)

	// Rate limit.
	time.Sleep(c.cfg.RateLimit)

	if err := c.db.TouchSourceCrawledAt(ctx, src.ID); err != nil {
		slog.Error("crawler: touching crawled_at", "error", err, "source_id", src.ID)
	}
}

// stubCrawl is the Phase 1 fallback for hosts without a parser.
func (c *Crawler) stubCrawl(ctx context.Context, j job) {
	item := &models.DownloadQueue{
		Type:     string(models.QueueTypeCrawl),
		URL:      j.source.URL,
		TargetID: &j.source.ID,
	}
	if err := c.db.EnqueueItem(ctx, item); err != nil {
		slog.Error("crawler: enqueueing stub crawl", "error", err, "source_id", j.source.ID)
		return
	}
	time.Sleep(c.cfg.RateLimit)
	if err := c.db.TouchSourceCrawledAt(ctx, j.source.ID); err != nil {
		slog.Error("crawler: touching crawled_at", "error", err, "source_id", j.source.ID)
	}
}

// fetchPage performs an HTTP GET and returns the response body as a string.
func (c *Crawler) fetchPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("crawler: building request: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("crawler: GET %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("crawler: GET %q returned %d", pageURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("crawler: reading body: %w", err)
	}
	return string(data), nil
}

// ensureGallery creates a gallery record for the source if one doesn't exist
// with the same title. Returns the gallery ID.
func (c *Crawler) ensureGallery(ctx context.Context, sourceID int64, title, sourceURL string) (int64, error) {
	// Check if a gallery with this title already exists for this source.
	galleries, err := c.db.ListGalleries(ctx, database.GalleryFilter{
		SourceID: &sourceID,
		Search:   &title,
		Limit:    1,
	})
	if err != nil {
		return 0, fmt.Errorf("checking existing gallery: %w", err)
	}

	if len(galleries) > 0 {
		return galleries[0].ID, nil
	}

	g := &models.Gallery{
		SourceID: &sourceID,
		Title:    &title,
		URL:      &sourceURL,
	}
	if err := c.db.CreateGallery(ctx, g); err != nil {
		return 0, fmt.Errorf("creating gallery: %w", err)
	}
	return g.ID, nil
}

// extractPostID extracts a post ID from a URL fragment (#postXXX) or query param (?p=XXX).
// Fragment takes precedence over query param.
// Returns empty string if neither is present.
func extractPostID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	// Try fragment first: #post66829071 -> 66829071
	fragment := strings.TrimPrefix(u.Fragment, "post")
	if fragment != "" && u.Fragment != "" {
		return fragment
	}

	// Fallback to query param: ?p=66829071
	if p := u.Query().Get("p"); p != "" {
		return p
	}

	return ""
}
