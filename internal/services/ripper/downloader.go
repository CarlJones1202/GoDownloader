// Package ripper handles downloading images and videos from external hosts.
// It provides a Ripper interface for per-provider scrapers and a core
// Downloader that streams HTTP responses directly to disk without buffering
// the entire body in memory.
package ripper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/carlj/godownload/internal/utils"
)

// ErrUnsupportedHost is returned when no ripper is registered for a URL's host.
var ErrUnsupportedHost = errors.New("ripper: unsupported host")

// Result holds the outcome of a successful download.
type Result struct {
	// LocalPath is the absolute path to the saved file.
	LocalPath string
	// Filename is the base name of the saved file.
	Filename string
	// FileHash is the hex-encoded SHA-256 of the file contents.
	FileHash string
	// ContentType is the MIME type reported by the server (or sniffed).
	ContentType string
}

// Ripper is implemented by per-provider scrapers. Given a page URL, Rip
// must resolve the direct media URL(s) and return them for downloading.
// A provider that hosts images directly (no scraping needed) should
// return the input URL unchanged in a single-element slice.
type Ripper interface {
	// Hosts returns the URL host patterns this ripper handles, e.g.
	// []string{"imagebam.com", "www.imagebam.com"}.
	Hosts() []string
	// Rip resolves direct download URL(s) from a gallery/image page URL.
	Rip(ctx context.Context, pageURL string) ([]string, error)
}

// Registry maps host strings to Rippers and dispatches downloads.
type Registry struct {
	mu        sync.RWMutex
	rippers   map[string]Ripper
	client    *http.Client
	destDir   string
	userAgent string
}

// Option is a functional option for Registry.
type Option func(*Registry)

// WithUserAgent sets the User-Agent header used for downloads.
func WithUserAgent(ua string) Option {
	return func(r *Registry) { r.userAgent = ua }
}

// NewRegistry creates a Registry that saves files to destDir.
func NewRegistry(destDir string, client *http.Client, opts ...Option) *Registry {
	if client == nil {
		client = utils.NewHTTPClient()
	}

	r := &Registry{
		rippers:   make(map[string]Ripper),
		client:    client,
		destDir:   destDir,
		userAgent: "GoDownload/1.0",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds a Ripper to the registry for all of its declared hosts.
func (r *Registry) Register(rip Ripper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, host := range rip.Hosts() {
		r.rippers[host] = rip
	}
}

// ripperFor returns the Ripper for the given URL, or nil if none is found.
func (r *Registry) ripperFor(rawURL string) (Ripper, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("ripper: parsing url %q: %w", rawURL, err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	host := strings.ToLower(u.Hostname())
	if rip, ok := r.rippers[host]; ok {
		return rip, nil
	}

	// Strip leading "www." and retry.
	bare := strings.TrimPrefix(host, "www.")
	if rip, ok := r.rippers[bare]; ok {
		return rip, nil
	}

	return nil, ErrUnsupportedHost
}

// Download resolves a page URL via its ripper and downloads all resulting
// media files to the destination directory. It returns one Result per file.
func (r *Registry) Download(ctx context.Context, pageURL string) ([]Result, error) {
	rip, err := r.ripperFor(pageURL)
	if err != nil {
		return nil, err
	}

	directURLs, err := rip.Rip(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("ripper: ripping %q: %w", pageURL, err)
	}

	results := make([]Result, 0, len(directURLs))
	for _, du := range directURLs {
		res, err := r.downloadDirect(ctx, du)
		if err != nil {
			slog.Warn("ripper: download failed", "url", du, "error", err)
			continue
		}
		results = append(results, res)
	}

	return results, nil
}

// downloadDirect streams a direct media URL to disk and computes its SHA-256.
// It never buffers the entire body in memory.
func (r *Registry) downloadDirect(ctx context.Context, rawURL string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("ripper: building request for %q: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ripper: GET %q: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("ripper: GET %q returned %d", rawURL, resp.StatusCode)
	}

	// Derive a filename from the URL path; fall back to a hash-based name.
	filename := filenameFromURL(rawURL)

	// Ensure the destination directory exists.
	if err := os.MkdirAll(r.destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ripper: creating dest dir %q: %w", r.destDir, err)
	}

	destPath := filepath.Join(r.destDir, filename)

	// Write to a temp file first so a partial download never leaves a corrupt file.
	tmp, err := os.CreateTemp(r.destDir, ".dl-*")
	if err != nil {
		return Result{}, fmt.Errorf("ripper: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Cleanup temp file on any error path.
	var writeErr error
	defer func() {
		if writeErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	// Tee: write to file AND feed the hasher simultaneously.
	w := io.MultiWriter(tmp, hasher)

	if _, writeErr = io.Copy(w, resp.Body); writeErr != nil {
		_ = tmp.Close()
		return Result{}, fmt.Errorf("ripper: writing %q: %w", tmpPath, writeErr)
	}

	if writeErr = tmp.Close(); writeErr != nil {
		return Result{}, fmt.Errorf("ripper: closing temp file: %w", writeErr)
	}

	// Atomic rename so readers never see a partial file.
	if writeErr = os.Rename(tmpPath, destPath); writeErr != nil {
		return Result{}, fmt.Errorf("ripper: renaming %q → %q: %w", tmpPath, destPath, writeErr)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	ct := resp.Header.Get("Content-Type")

	return Result{
		LocalPath:   destPath,
		Filename:    filename,
		FileHash:    hash,
		ContentType: ct,
	}, nil
}

// filenameFromURL extracts a safe filename from the last path segment of a URL.
// If the segment is empty or unparseable it returns a placeholder.
func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "download"
	}

	base := filepath.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "download"
	}

	// Sanitise: replace any characters that are not alphanumeric, dash,
	// underscore, or dot with an underscore.
	var sb strings.Builder
	for _, c := range base {
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' {
			sb.WriteRune(c)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}
