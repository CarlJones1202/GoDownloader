package workers

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

const (
	// DefaultTrickplayInterval is how often a frame is captured (seconds).
	DefaultTrickplayInterval = 10
	// DefaultTrickplayCols is how many thumbnails per row in the sprite sheet.
	DefaultTrickplayCols = 5
	// DefaultTrickplayThumbW is the width of each tile in the sprite sheet.
	DefaultTrickplayThumbW = 160
	// DefaultTrickplayThumbH is the height of each tile in the sprite sheet.
	DefaultTrickplayThumbH = 90
)

// TrickplayWorker generates trickplay data (sprite sheets + WebVTT) for video files.
// It requires ffmpeg to be available on PATH for frame extraction.
type TrickplayWorker struct {
	db       *database.DB
	videoDir string
	thumbDir string
	interval int // seconds between frames
	cols     int // tiles per row in sprite sheet
	tileW    int // tile width in pixels
	tileH    int // tile height in pixels
}

// TrickplayOption is a functional option for TrickplayWorker.
type TrickplayOption func(*TrickplayWorker)

// WithTrickplayInterval sets the frame capture interval in seconds.
func WithTrickplayInterval(s int) TrickplayOption {
	return func(tw *TrickplayWorker) { tw.interval = s }
}

// WithTrickplayCols sets the number of columns in the sprite sheet.
func WithTrickplayCols(n int) TrickplayOption {
	return func(tw *TrickplayWorker) { tw.cols = n }
}

// WithTrickplayTileSize sets the width and height of each tile.
func WithTrickplayTileSize(w, h int) TrickplayOption {
	return func(tw *TrickplayWorker) { tw.tileW = w; tw.tileH = h }
}

// NewTrickplayWorker creates a TrickplayWorker.
func NewTrickplayWorker(db *database.DB, videoDir, thumbDir string, opts ...TrickplayOption) *TrickplayWorker {
	tw := &TrickplayWorker{
		db:       db,
		videoDir: videoDir,
		thumbDir: thumbDir,
		interval: DefaultTrickplayInterval,
		cols:     DefaultTrickplayCols,
		tileW:    DefaultTrickplayThumbW,
		tileH:    DefaultTrickplayThumbH,
	}
	for _, opt := range opts {
		opt(tw)
	}
	return tw
}

// GenerateForVideo produces a sprite sheet and a WebVTT file for the given
// video image record.
//
// Output files:
//   - <thumbDir>/<base>_sprites.jpg  — sprite sheet
//   - <thumbDir>/<base>_sprites.vtt  — WebVTT trickplay file
//
// Requires ffmpeg on PATH. Returns ErrFFmpegNotFound if ffmpeg is missing.
func (tw *TrickplayWorker) GenerateForVideo(ctx context.Context, img *models.Image) error {
	if !img.IsVideo {
		return nil
	}

	videoPath := filepath.Join(tw.videoDir, img.Filename)

	// Check ffmpeg is available.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ErrFFmpegNotFound
	}

	base := strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))
	spriteFile := base + "_sprites.jpg"
	vttFile := base + "_sprites.vtt"
	spritePath := filepath.Join(tw.thumbDir, spriteFile)
	vttPath := filepath.Join(tw.thumbDir, vttFile)

	// Idempotency: skip if both files already exist.
	if fileExists(spritePath) && fileExists(vttPath) {
		return nil
	}

	if err := os.MkdirAll(tw.thumbDir, 0o755); err != nil {
		return fmt.Errorf("trickplay: creating thumb dir: %w", err)
	}

	// Extract frames into a temporary directory.
	tmpDir, err := os.MkdirTemp("", "trickplay-*")
	if err != nil {
		return fmt.Errorf("trickplay: creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := tw.extractFrames(ctx, videoPath, tmpDir); err != nil {
		return err
	}

	frames, err := filepath.Glob(filepath.Join(tmpDir, "frame*.jpg"))
	if err != nil || len(frames) == 0 {
		return fmt.Errorf("trickplay: no frames extracted from %q", videoPath)
	}

	// Build the sprite sheet.
	if err := tw.buildSpriteSheet(frames, spritePath); err != nil {
		return err
	}

	// Build the WebVTT file.
	if err := tw.buildVTT(frames, spriteFile, vttPath); err != nil {
		return err
	}

	slog.Info("trickplay: generated",
		"image_id", img.ID,
		"frames", len(frames),
		"sprite", spritePath,
		"vtt", vttPath,
	)
	return nil
}

// extractFrames runs ffmpeg to capture one frame every tw.interval seconds.
func (tw *TrickplayWorker) extractFrames(ctx context.Context, videoPath, outDir string) error {
	// ffmpeg -i <input> -vf fps=1/<interval>,scale=<w>:<h> <outDir>/frame%04d.jpg
	args := []string{
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=1/%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			tw.interval, tw.tileW, tw.tileH, tw.tileW, tw.tileH),
		"-q:v", "3",
		filepath.Join(outDir, "frame%04d.jpg"),
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("trickplay: ffmpeg error: %w\n%s", err, out)
	}
	return nil
}

// buildSpriteSheet stitches frame images into a single JPEG sprite sheet.
func (tw *TrickplayWorker) buildSpriteSheet(frames []string, outPath string) error {
	rows := (len(frames) + tw.cols - 1) / tw.cols
	sheetW := tw.cols * tw.tileW
	sheetH := rows * tw.tileH

	sheet := image.NewRGBA(image.Rect(0, 0, sheetW, sheetH))

	for i, framePath := range frames {
		tile, err := loadJPEG(framePath)
		if err != nil {
			return fmt.Errorf("trickplay: loading frame %q: %w", framePath, err)
		}

		col := i % tw.cols
		row := i / tw.cols
		x := col * tw.tileW
		y := row * tw.tileH

		dst := image.Rect(x, y, x+tw.tileW, y+tw.tileH)
		draw.Draw(sheet, dst, tile, tile.Bounds().Min, draw.Src)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("trickplay: creating sprite sheet %q: %w", outPath, err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, sheet, &jpeg.Options{Quality: 80}); err != nil {
		return fmt.Errorf("trickplay: encoding sprite sheet: %w", err)
	}

	return nil
}

// buildVTT writes a WebVTT file that maps time ranges to sprite sheet regions.
func (tw *TrickplayWorker) buildVTT(frames []string, spriteFile, vttPath string) error {
	out, err := os.Create(vttPath)
	if err != nil {
		return fmt.Errorf("trickplay: creating vtt %q: %w", vttPath, err)
	}
	defer out.Close()

	if _, err := fmt.Fprintln(out, "WEBVTT"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	for i := range frames {
		start := time.Duration(i*tw.interval) * time.Second
		end := time.Duration((i+1)*tw.interval) * time.Second

		col := i % tw.cols
		row := i / tw.cols
		x := col * tw.tileW
		y := row * tw.tileH

		entry := fmt.Sprintf("%s --> %s\n%s#xywh=%d,%d,%d,%d\n\n",
			formatVTTTime(start),
			formatVTTTime(end),
			spriteFile,
			x, y, tw.tileW, tw.tileH,
		)

		if _, err := fmt.Fprint(out, entry); err != nil {
			return fmt.Errorf("trickplay: writing vtt entry: %w", err)
		}
	}

	return nil
}

// formatVTTTime formats a Duration as HH:MM:SS.mmm for WebVTT.
func formatVTTTime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// loadJPEG opens and decodes a JPEG file.
func loadJPEG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jpeg.Decode(f)
}

// fileExists reports whether a file exists at path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ErrFFmpegNotFound is returned when ffmpeg is not available on PATH.
var ErrFFmpegNotFound = fmt.Errorf("trickplay: ffmpeg not found on PATH")
