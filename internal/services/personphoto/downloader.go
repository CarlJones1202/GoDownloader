// Package personphoto downloads and manages person profile photos.
// Photos are stored locally at data/person_photos/{personID}/{sha256}.{ext}
// and served via /data/person_photos/... static route.
package personphoto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Downloader downloads person photos from remote URLs and saves them locally.
type Downloader struct {
	baseDir   string // e.g. "data/person_photos"
	client    *http.Client
	userAgent string
}

// NewDownloader creates a person photo downloader.
func NewDownloader(baseDir string, client *http.Client, userAgent string) *Downloader {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	return &Downloader{
		baseDir:   baseDir,
		client:    client,
		userAgent: userAgent,
	}
}

// Download fetches a photo from the given URL and saves it to the person's
// photo directory. Returns the web-accessible path (e.g.
// "/data/person_photos/42/abc123.jpg") or an error.
//
// If a file with the same SHA-256 hash already exists, it is not re-downloaded
// and the existing path is returned.
func (d *Downloader) Download(ctx context.Context, photoURL string, personID int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photoURL, nil)
	if err != nil {
		return "", fmt.Errorf("building photo request: %w", err)
	}
	req.Header.Set("User-Agent", d.userAgent)
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading photo %q: %w", photoURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("photo %q returned %d", photoURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading photo %q: %w", photoURL, err)
	}

	if len(body) == 0 {
		return "", fmt.Errorf("photo %q: empty response body", photoURL)
	}

	// SHA-256 hash for filename (dedup + stable naming).
	hash := sha256.Sum256(body)
	hashStr := hex.EncodeToString(hash[:])

	// Determine extension from Content-Type header.
	ext := extFromContentType(resp.Header.Get("Content-Type"))
	if ext == "" {
		// Fallback: try to get extension from URL.
		ext = extFromURL(photoURL)
	}
	if ext == "" {
		ext = ".jpg" // ultimate fallback
	}

	// Ensure person directory exists.
	personDir := filepath.Join(d.baseDir, fmt.Sprintf("%d", personID))
	if err := os.MkdirAll(personDir, 0o755); err != nil {
		return "", fmt.Errorf("creating person photo dir: %w", err)
	}

	filename := hashStr + ext
	diskPath := filepath.Join(personDir, filename)

	// Skip if file already exists (same hash = same content).
	if _, err := os.Stat(diskPath); err == nil {
		slog.Debug("person photo already exists", "person_id", personID, "file", filename)
		webPath := fmt.Sprintf("/data/person_photos/%d/%s", personID, filename)
		return webPath, nil
	}

	if err := os.WriteFile(diskPath, body, 0o644); err != nil {
		return "", fmt.Errorf("writing photo to disk: %w", err)
	}

	webPath := fmt.Sprintf("/data/person_photos/%d/%s", personID, filename)
	slog.Info("downloaded person photo", "person_id", personID, "url", photoURL, "path", webPath, "size", len(body))
	return webPath, nil
}

// DownloadAll downloads multiple photos for a person and returns the list
// of web-accessible paths. Errors on individual photos are logged but don't
// stop processing — the function returns as many paths as it can.
func (d *Downloader) DownloadAll(ctx context.Context, photoURLs []string, personID int64) []string {
	var paths []string
	for _, u := range photoURLs {
		if u == "" {
			continue
		}
		path, err := d.Download(ctx, u, personID)
		if err != nil {
			slog.Warn("failed to download person photo",
				"person_id", personID, "url", u, "error", err)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// extFromContentType maps a Content-Type to a file extension.
func extFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "image/jpeg"), strings.Contains(ct, "image/jpg"):
		return ".jpg"
	case strings.Contains(ct, "image/png"):
		return ".png"
	case strings.Contains(ct, "image/webp"):
		return ".webp"
	case strings.Contains(ct, "image/gif"):
		return ".gif"
	case strings.Contains(ct, "image/avif"):
		return ".avif"
	default:
		return ""
	}
}

// extFromURL extracts a file extension from a URL path.
func extFromURL(rawURL string) string {
	// Strip query string.
	if idx := strings.IndexByte(rawURL, '?'); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	ext := filepath.Ext(rawURL)
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif":
		return ext
	default:
		return ""
	}
}
