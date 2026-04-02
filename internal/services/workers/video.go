package workers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

// VideoWorker extracts metadata from video files using ffprobe and generates
// thumbnails using ffmpeg. It updates the database with width, height, and
// duration for each processed video.
type VideoWorker struct {
	db       *database.DB
	videoDir string
	thumbDir string
}

// VideoOption is a functional option for VideoWorker.
type VideoOption func(*VideoWorker)

// NewVideoWorker creates a VideoWorker.
func NewVideoWorker(db *database.DB, videoDir, thumbDir string, opts ...VideoOption) *VideoWorker {
	vw := &VideoWorker{
		db:       db,
		videoDir: videoDir,
		thumbDir: thumbDir,
	}
	for _, opt := range opts {
		opt(vw)
	}
	return vw
}

// VideoMeta holds the metadata extracted from a video file via ffprobe.
type VideoMeta struct {
	Width    int
	Height   int
	Duration float64 // seconds
}

// ProcessVideo extracts metadata, updates the DB, and generates a thumbnail
// for the given video image record.
func (vw *VideoWorker) ProcessVideo(ctx context.Context, img *models.Image) error {
	if !img.IsVideo {
		return nil
	}

	videoPath := filepath.Join(vw.videoDir, img.Filename)

	// 1. Extract metadata via ffprobe.
	meta, err := vw.ffprobeMetadata(ctx, videoPath)
	if err != nil {
		slog.Warn("video-worker: ffprobe failed, skipping metadata",
			"image_id", img.ID, "error", err)
		// Non-fatal: still try to generate thumbnail.
	} else {
		durSec := int(meta.Duration)
		if err := vw.db.UpdateImageVideoMeta(ctx, img.ID, meta.Width, meta.Height, durSec); err != nil {
			slog.Warn("video-worker: updating video metadata",
				"image_id", img.ID, "error", err)
		} else {
			slog.Info("video-worker: metadata saved",
				"image_id", img.ID,
				"width", meta.Width,
				"height", meta.Height,
				"duration", durSec,
			)
		}
	}

	// 2. Generate a thumbnail at the video midpoint.
	if err := vw.generateThumbnail(ctx, img, meta); err != nil {
		slog.Warn("video-worker: thumbnail generation failed",
			"image_id", img.ID, "error", err)
		// Non-fatal.
	}

	return nil
}

// ffprobeMetadata shells out to ffprobe to extract width, height, and duration
// from the first video stream.
//
// Command:
//
//	ffprobe -v error -select_streams v:0 \
//	  -show_entries stream=width,height,duration \
//	  -of csv=p=0 <path>
//
// Parses CSV output like "1920,1080,123.456". If stream-level duration is 0
// or missing, falls back to container-level duration.
func (vw *VideoWorker) ffprobeMetadata(ctx context.Context, videoPath string) (*VideoMeta, error) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil, fmt.Errorf("video-worker: ffprobe not found on PATH")
	}

	// Try stream-level metadata first.
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration",
		"-of", "csv=p=0",
		videoPath,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("video-worker: ffprobe stream: %w", err)
	}

	meta, err := parseFFprobeCSV(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, fmt.Errorf("video-worker: parsing ffprobe output %q: %w", string(out), err)
	}

	// If duration is 0 or missing from the stream, try container-level duration.
	if meta.Duration <= 0 {
		dur, err := vw.ffprobeContainerDuration(ctx, videoPath)
		if err == nil && dur > 0 {
			meta.Duration = dur
		}
	}

	return meta, nil
}

// ffprobeContainerDuration extracts duration from the container format as a fallback.
//
// Command:
//
//	ffprobe -v error -show_entries format=duration -of csv=p=0 <path>
func (vw *VideoWorker) ffprobeContainerDuration(ctx context.Context, videoPath string) (float64, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		videoPath,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("video-worker: ffprobe format duration: %w", err)
	}

	dur, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("video-worker: parsing duration %q: %w", string(out), err)
	}

	return dur, nil
}

// parseFFprobeCSV parses ffprobe CSV output "width,height,duration" or
// "width,height" (duration may be missing or "N/A").
func parseFFprobeCSV(csv string) (*VideoMeta, error) {
	if csv == "" {
		return nil, fmt.Errorf("empty ffprobe output")
	}

	parts := strings.Split(csv, ",")
	if len(parts) < 2 {
		return nil, fmt.Errorf("expected at least 2 fields, got %d", len(parts))
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("parsing width %q: %w", parts[0], err)
	}

	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("parsing height %q: %w", parts[1], err)
	}

	var duration float64
	if len(parts) >= 3 {
		durStr := strings.TrimSpace(parts[2])
		if durStr != "" && durStr != "N/A" {
			duration, _ = strconv.ParseFloat(durStr, 64) // ignore parse errors; 0 is fine
		}
	}

	return &VideoMeta{
		Width:    width,
		Height:   height,
		Duration: duration,
	}, nil
}

// generateThumbnail captures a single frame at the video midpoint and saves
// it as a JPEG thumbnail.
//
// Output: <thumbDir>/<base>_thumb.jpg
//
// Command:
//
//	ffmpeg -ss <midpoint> -i <path> -frames:v 1 -q:v 3 <output>
func (vw *VideoWorker) generateThumbnail(ctx context.Context, img *models.Image, meta *VideoMeta) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ErrFFmpegNotFound
	}

	videoPath := filepath.Join(vw.videoDir, img.Filename)

	ext := filepath.Ext(img.Filename)
	base := strings.TrimSuffix(img.Filename, ext)
	thumbFilename := base + "_thumb.jpg"
	thumbPath := filepath.Join(vw.thumbDir, thumbFilename)

	// Idempotency: skip if already exists.
	if fileExists(thumbPath) {
		return nil
	}

	if err := os.MkdirAll(vw.thumbDir, 0o755); err != nil {
		return fmt.Errorf("video-worker: creating thumb dir: %w", err)
	}

	// Seek to midpoint if we know the duration, otherwise default to 2s.
	seekSec := 2.0
	if meta != nil && meta.Duration > 4 {
		seekSec = meta.Duration / 2
	}

	args := []string{
		"-ss", fmt.Sprintf("%.2f", seekSec),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "3",
		"-y", // overwrite if exists
		thumbPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("video-worker: ffmpeg thumbnail: %w\n%s", err, out)
	}

	slog.Info("video-worker: thumbnail generated",
		"image_id", img.ID,
		"path", thumbPath,
	)

	return nil
}
