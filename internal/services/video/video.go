// Package video provides site-specific video rippers for downloading videos
// from TnaFlix, YouTube, Pornhub, PMVHaven, and other supported sites.
// Each ripper extracts the direct video URL (and title) from a page, and the
// Downloader handles the actual HTTP fetch + hash-based file naming.
package video

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
)

// RipResult holds the output of a site-specific ripper: a direct download URL
// and the video title extracted from the page.
type RipResult struct {
	DirectURL string
	Title     string
}

// Result holds the outcome of a successful video download.
type Result struct {
	LocalPath string
	Filename  string
	FileHash  string
	Title     string
}

// Ripper is implemented by per-site video extractors. Given a page URL, it
// resolves the direct video download URL and title.
type Ripper interface {
	// Hosts returns URL host patterns this ripper handles.
	Hosts() []string
	// Rip extracts the direct video URL and title from a page URL.
	Rip(ctx context.Context, pageURL string) (*RipResult, error)
}

// Registry maps host strings to video Rippers.
type Registry struct {
	rippers   map[string]Ripper
	client    *http.Client
	destDir   string
	userAgent string
}

// NewRegistry creates a video ripper Registry that saves to destDir.
func NewRegistry(destDir string, client *http.Client, userAgent string) *Registry {
	if client == nil {
		client = &http.Client{}
	}
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	return &Registry{
		rippers:   make(map[string]Ripper),
		client:    client,
		destDir:   destDir,
		userAgent: userAgent,
	}
}

// Register adds a Ripper to the registry for all of its declared hosts.
func (r *Registry) Register(rip Ripper) {
	for _, host := range rip.Hosts() {
		r.rippers[strings.ToLower(host)] = rip
	}
}

// RipperFor returns the Ripper for the given URL host, or nil.
func (r *Registry) RipperFor(rawURL string) Ripper {
	host := hostFromURL(rawURL)
	if rip, ok := r.rippers[host]; ok {
		return rip
	}
	bare := strings.TrimPrefix(host, "www.")
	if rip, ok := r.rippers[bare]; ok {
		return rip
	}
	return nil
}

// Download rips the video URL from the page and downloads the file.
func (r *Registry) Download(ctx context.Context, pageURL string) (*Result, error) {
	rip := r.RipperFor(pageURL)
	if rip == nil {
		return nil, fmt.Errorf("video: no ripper for %q", pageURL)
	}

	ripResult, err := rip.Rip(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("video: ripping %q: %w", pageURL, err)
	}

	// yt-dlp returns file:// URLs for already-downloaded local files.
	if strings.HasPrefix(ripResult.DirectURL, "file://") {
		result, err := r.moveLocalFile(ripResult.DirectURL)
		if err != nil {
			return nil, err
		}
		result.Title = ripResult.Title
		return result, nil
	}

	result, err := r.downloadDirect(ctx, ripResult.DirectURL, pageURL)
	if err != nil {
		return nil, fmt.Errorf("video: downloading %q: %w", ripResult.DirectURL, err)
	}
	result.Title = ripResult.Title

	return result, nil
}

// moveLocalFile moves a file from a temporary location (file:// URL) into the
// destination directory with hash-based naming. Used by the yt-dlp ripper.
func (r *Registry) moveLocalFile(fileURL string) (*Result, error) {
	localPath := strings.TrimPrefix(fileURL, "file://")

	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("video: opening local file %q: %w", localPath, err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		f.Close()
		return nil, fmt.Errorf("video: hashing local file: %w", err)
	}
	f.Close()

	hash := hex.EncodeToString(hasher.Sum(nil))
	ext := filepath.Ext(localPath)
	if ext == "" {
		ext = ".mp4"
	}
	filename := hash + ext

	if err := os.MkdirAll(r.destDir, 0o755); err != nil {
		return nil, fmt.Errorf("video: creating dest dir: %w", err)
	}

	destPath := filepath.Join(r.destDir, filename)

	// If file already exists, just remove the source.
	if _, err := os.Stat(destPath); err == nil {
		_ = os.Remove(localPath)
		// Also clean up parent temp dir if empty.
		_ = os.Remove(filepath.Dir(localPath))
	} else {
		if err := os.Rename(localPath, destPath); err != nil {
			// Rename can fail across filesystems; fall back to copy.
			if cpErr := copyFile(localPath, destPath); cpErr != nil {
				return nil, fmt.Errorf("video: moving file: %w", cpErr)
			}
			_ = os.Remove(localPath)
		}
		// Clean up temp dir.
		_ = os.Remove(filepath.Dir(localPath))
	}

	slog.Info("video: moved local file", "filename", filename, "hash", hash)

	return &Result{
		LocalPath: destPath,
		Filename:  filename,
		FileHash:  hash,
	}, nil
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// downloadDirect streams the video file to disk with hash-based naming.
func (r *Registry) downloadDirect(ctx context.Context, videoURL, referer string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Referer", referer)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %q: %w", videoURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %q returned %d", videoURL, resp.StatusCode)
	}

	if err := os.MkdirAll(r.destDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating dest dir: %w", err)
	}

	// Write to a temp file so partial downloads don't leave corrupt files.
	tmp, err := os.CreateTemp(r.destDir, ".vdl-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	var writeErr error
	defer func() {
		if writeErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	w := io.MultiWriter(tmp, hasher)

	if _, writeErr = io.Copy(w, resp.Body); writeErr != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("writing video: %w", writeErr)
	}

	if writeErr = tmp.Close(); writeErr != nil {
		return nil, fmt.Errorf("closing temp file: %w", writeErr)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	ext := extensionFromVideoURL(videoURL)
	filename := hash + ext
	destPath := filepath.Join(r.destDir, filename)

	// If file already exists (duplicate), just remove the temp.
	if _, err := os.Stat(destPath); err == nil {
		_ = os.Remove(tmpPath)
		slog.Debug("video: file already exists", "filename", filename)
	} else {
		if writeErr = os.Rename(tmpPath, destPath); writeErr != nil {
			return nil, fmt.Errorf("renaming %q -> %q: %w", tmpPath, destPath, writeErr)
		}
	}

	slog.Info("video: downloaded", "filename", filename, "hash", hash)

	return &Result{
		LocalPath: destPath,
		Filename:  filename,
		FileHash:  hash,
	}, nil
}

// IsVideoURL checks if a URL points to a video based on domain or extension.
func IsVideoURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	// Check known video file extensions.
	for _, ext := range []string{".mp4", ".mkv", ".webm", ".avi", ".mov", ".wmv", ".flv"} {
		if strings.HasSuffix(lower, ext) || strings.Contains(lower, ext+"?") {
			return true
		}
	}
	// Check known video site domains.
	host := hostFromURL(rawURL)
	bare := strings.TrimPrefix(host, "www.")
	switch bare {
	case "tnaflix.com", "pornhub.com", "pmvhaven.com",
		"youtube.com", "youtu.be":
		return true
	}
	return false
}

// hostFromURL extracts the lowercase hostname from a URL.
func hostFromURL(rawURL string) string {
	// Fast path: avoid url.Parse for simple cases.
	idx := strings.Index(rawURL, "://")
	if idx < 0 {
		return ""
	}
	rest := rawURL[idx+3:]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		rest = rest[:i]
	}
	return strings.ToLower(rest)
}

// extensionFromVideoURL extracts the file extension from a URL path.
func extensionFromVideoURL(rawURL string) string {
	// Strip query string.
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		rawURL = rawURL[:i]
	}
	ext := filepath.Ext(rawURL)
	switch strings.ToLower(ext) {
	case ".mp4", ".webm", ".mkv", ".avi", ".mov":
		return ext
	default:
		return ".mp4" // default to mp4
	}
}
