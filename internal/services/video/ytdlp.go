package video

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// YtDlpRipper handles YouTube, Pornhub, and other sites supported by yt-dlp.
// It shells out to the yt-dlp CLI which must be available on PATH.
type YtDlpRipper struct {
	cookiesFile string // optional Netscape cookies file for YouTube auth
}

// NewYtDlpRipper creates a YtDlpRipper. cookiesFile is optional — pass ""
// to skip cookies.
func NewYtDlpRipper(cookiesFile string) *YtDlpRipper {
	return &YtDlpRipper{cookiesFile: cookiesFile}
}

func (r *YtDlpRipper) Hosts() []string {
	return []string{
		"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be",
		"pornhub.com", "www.pornhub.com",
	}
}

// Rip uses yt-dlp to download the video to a temp directory, then returns
// a RipResult whose DirectURL is actually a local file path (prefixed with
// "file://"). The video.Registry.Download detects this and skips the HTTP
// download step, just moving the file.
func (r *YtDlpRipper) Rip(ctx context.Context, pageURL string) (*RipResult, error) {
	slog.Info("ytdlp: ripping", "url", pageURL)

	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return nil, fmt.Errorf("ytdlp: yt-dlp not found on PATH")
	}

	tempDir, err := os.MkdirTemp("", "ytdlp-*")
	if err != nil {
		return nil, fmt.Errorf("ytdlp: creating temp dir: %w", err)
	}

	outputTemplate := filepath.Join(tempDir, "%(id)s.%(ext)s")

	// Step 1: Get metadata (title + expected filename).
	metaArgs := []string{
		"--get-title", "--get-filename",
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--no-playlist",
		"--merge-output-format", "mp4",
		"-o", outputTemplate,
	}
	if r.cookiesFile != "" {
		if _, err := os.Stat(r.cookiesFile); err == nil {
			metaArgs = append(metaArgs, "--cookies", r.cookiesFile)
		}
	}
	metaArgs = append(metaArgs, pageURL)

	metaCmd := exec.CommandContext(ctx, "yt-dlp", metaArgs...)
	metaOut, err := metaCmd.Output()
	if err != nil {
		// Clean up temp dir on error.
		os.RemoveAll(tempDir)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ytdlp: metadata failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ytdlp: metadata failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(metaOut)), "\n")
	if len(lines) < 2 {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("ytdlp: unexpected metadata output: %s", string(metaOut))
	}
	title := strings.TrimSpace(lines[0])
	expectedPath := strings.TrimSpace(lines[len(lines)-1])

	slog.Info("ytdlp: video metadata", "title", title, "path", expectedPath)

	// Step 2: Download.
	dlArgs := []string{
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--no-playlist",
		"--merge-output-format", "mp4",
		"-o", outputTemplate,
	}
	if r.cookiesFile != "" {
		if _, err := os.Stat(r.cookiesFile); err == nil {
			dlArgs = append(dlArgs, "--cookies", r.cookiesFile)
		}
	}
	dlArgs = append(dlArgs, pageURL)

	dlCmd := exec.CommandContext(ctx, "yt-dlp", dlArgs...)

	// Stream stderr to logs.
	stderr, _ := dlCmd.StderrPipe()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Debug("ytdlp", "output", scanner.Text())
		}
	}()

	if err := dlCmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("ytdlp: download failed: %w", err)
	}

	// Verify file exists.
	if _, err := os.Stat(expectedPath); err != nil {
		// Try to find any video file in the temp dir as fallback.
		entries, _ := os.ReadDir(tempDir)
		for _, e := range entries {
			if !e.IsDir() && isVideoExt(filepath.Ext(e.Name())) {
				expectedPath = filepath.Join(tempDir, e.Name())
				break
			}
		}
		if _, err := os.Stat(expectedPath); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("ytdlp: downloaded file not found: %w", err)
		}
	}

	// Return a file:// URL so the registry knows this is already local.
	return &RipResult{
		DirectURL: "file://" + expectedPath,
		Title:     title,
	}, nil
}

// isVideoExt checks common video file extensions.
func isVideoExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov":
		return true
	}
	return false
}
