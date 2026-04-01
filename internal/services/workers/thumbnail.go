// Package workers contains background workers that run post-download
// processing tasks: thumbnail generation, color extraction, and trickplay.
package workers

import (
	"context"
	"fmt"
	"image"
	_ "image/gif" // register gif decoder
	"image/jpeg"
	_ "image/png" // register png decoder
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

const (
	// DefaultThumbWidth is the longest edge of a generated thumbnail in pixels.
	DefaultThumbWidth = 320
	// DefaultThumbQuality is the JPEG quality used for saved thumbnails (0-100).
	DefaultThumbQuality = 80
)

// ThumbnailWorker generates JPEG thumbnails for downloaded images.
type ThumbnailWorker struct {
	db       *database.DB
	srcDir   string // where original images live
	thumbDir string // where thumbnails are saved
	width    int    // max width in pixels
	quality  int    // JPEG quality
}

// ThumbnailOption is a functional option for ThumbnailWorker.
type ThumbnailOption func(*ThumbnailWorker)

// WithThumbWidth overrides the default thumbnail width.
func WithThumbWidth(w int) ThumbnailOption {
	return func(tw *ThumbnailWorker) { tw.width = w }
}

// WithThumbQuality overrides the default JPEG quality (1-100).
func WithThumbQuality(q int) ThumbnailOption {
	return func(tw *ThumbnailWorker) { tw.quality = q }
}

// NewThumbnailWorker creates a ThumbnailWorker.
func NewThumbnailWorker(db *database.DB, srcDir, thumbDir string, opts ...ThumbnailOption) *ThumbnailWorker {
	tw := &ThumbnailWorker{
		db:       db,
		srcDir:   srcDir,
		thumbDir: thumbDir,
		width:    DefaultThumbWidth,
		quality:  DefaultThumbQuality,
	}
	for _, opt := range opts {
		opt(tw)
	}
	return tw
}

// GenerateForImage creates a thumbnail for the given image record and updates
// the record's Width/Height fields in the database.
// It is idempotent — if the thumbnail file already exists it is left unchanged.
func (tw *ThumbnailWorker) GenerateForImage(ctx context.Context, img *models.Image) error {
	srcPath := filepath.Join(tw.srcDir, img.Filename)

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("thumbnail: opening %q: %w", srcPath, err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("thumbnail: decoding %q: %w", srcPath, err)
	}

	bounds := src.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	// Update image dimensions while we have them.
	if img.Width == nil || img.Height == nil {
		if err := tw.updateDimensions(ctx, img.ID, origW, origH); err != nil {
			slog.Warn("thumbnail: updating dimensions", "image_id", img.ID, "error", err)
		}
	}

	thumbFilename := thumbnailFilename(img.Filename)
	thumbPath := filepath.Join(tw.thumbDir, thumbFilename)

	// Idempotency: skip if already generated.
	if _, err := os.Stat(thumbPath); err == nil {
		return nil
	}

	// Compute thumbnail dimensions maintaining aspect ratio.
	thumbW, thumbH := scaledDimensions(origW, origH, tw.width)

	thumb := resizeNearest(src, thumbW, thumbH)

	if err := os.MkdirAll(tw.thumbDir, 0o755); err != nil {
		return fmt.Errorf("thumbnail: creating thumb dir: %w", err)
	}

	out, err := os.Create(thumbPath)
	if err != nil {
		return fmt.Errorf("thumbnail: creating %q: %w", thumbPath, err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: tw.quality}); err != nil {
		return fmt.Errorf("thumbnail: encoding %q: %w", thumbPath, err)
	}

	slog.Info("thumbnail: generated", "image_id", img.ID, "path", thumbPath)
	return nil
}

// updateDimensions persists decoded image dimensions back to the database.
func (tw *ThumbnailWorker) updateDimensions(ctx context.Context, id int64, w, h int) error {
	_, err := tw.db.ExecContext(ctx,
		`UPDATE images SET width = ?, height = ? WHERE id = ?`, w, h, id,
	)
	if err != nil {
		return fmt.Errorf("thumbnail: updating dimensions for image %d: %w", id, err)
	}
	return nil
}

// thumbnailFilename returns the thumbnail filename for a given image filename.
// e.g. "photo.jpg" → "photo_thumb.jpg"
func thumbnailFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	// Always output JPEG thumbnails.
	return base + "_thumb.jpg"
}

// scaledDimensions returns (width, height) such that the longest edge
// is at most maxWidth while preserving the aspect ratio.
func scaledDimensions(origW, origH, maxWidth int) (int, int) {
	if origW <= maxWidth {
		return origW, origH
	}
	ratio := float64(origH) / float64(origW)
	return maxWidth, int(float64(maxWidth) * ratio)
}

// resizeNearest resizes src to (w, h) using nearest-neighbour sampling.
// This avoids any external dependency while producing acceptable thumbnail quality.
func resizeNearest(src image.Image, w, h int) image.Image {
	srcB := src.Bounds()
	srcW := srcB.Dx()
	srcH := srcB.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		srcY := (y * srcH) / h
		for x := range w {
			srcX := (x * srcW) / w
			dst.Set(x, y, src.At(srcB.Min.X+srcX, srcB.Min.Y+srcY))
		}
	}

	return dst
}
