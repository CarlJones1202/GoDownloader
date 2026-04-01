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

// SourceParser extracts gallery and image information from a crawled source page.
// Each forum/site type has its own implementation.
type SourceParser interface {
	// Hosts returns URL host patterns this parser handles,
	// e.g. []string{"vipergirls.to", "www.vipergirls.to"}.
	Hosts() []string
	// Parse receives the page HTML and returns the discovered image page URLs
	// grouped by gallery title. The map key is the gallery title (or "" for
	// ungrouped images).
	Parse(ctx context.Context, body, pageURL string) (map[string][]string, error)
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

	// Fetch the page.
	body, err := c.fetchPage(ctx, src.URL)
	if err != nil {
		slog.Error("crawler: fetching source page",
			"error", err,
			"source_id", src.ID,
		)
		return
	}

	// Parse galleries from the page HTML.
	galleries, err := parser.Parse(ctx, body, src.URL)
	if err != nil {
		slog.Error("crawler: parsing source page",
			"error", err,
			"source_id", src.ID,
		)
		return
	}

	// Create galleries and enqueue image downloads.
	totalImages := 0
	for title, imageURLs := range galleries {
		galleryID, err := c.ensureGallery(ctx, src.ID, title, src.URL)
		if err != nil {
			slog.Error("crawler: creating gallery",
				"error", err,
				"title", title,
			)
			continue
		}

		for _, imgURL := range imageURLs {
			item := &models.DownloadQueue{
				Type:     string(models.QueueTypeImage),
				URL:      imgURL,
				TargetID: &galleryID,
			}
			if err := c.db.EnqueueItem(ctx, item); err != nil {
				slog.Error("crawler: enqueueing image",
					"error", err,
					"url", imgURL,
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
