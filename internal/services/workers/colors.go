package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

const (
	// DefaultColorClusters is the number of dominant colors to extract.
	DefaultColorClusters = 5
	// defaultKMeansIter is the maximum number of K-means iterations.
	defaultKMeansIter = 20
	// colorSampleStep controls how many pixels are skipped during sampling.
	// A value of 4 samples roughly 1/16 of the image, keeping extraction fast.
	colorSampleStep = 4
)

// RGB holds a colour as three uint8 channels.
type RGB struct {
	R, G, B uint8
}

// ColorWorker extracts dominant colours from image files using K-means clustering
// and persists the result as a JSON array in images.dominant_colors.
type ColorWorker struct {
	db       *database.DB
	srcDir   string
	clusters int
}

// ColorOption is a functional option for ColorWorker.
type ColorOption func(*ColorWorker)

// WithColorClusters overrides the number of dominant colors to extract.
func WithColorClusters(n int) ColorOption {
	return func(cw *ColorWorker) { cw.clusters = n }
}

// NewColorWorker creates a ColorWorker.
func NewColorWorker(db *database.DB, srcDir string, opts ...ColorOption) *ColorWorker {
	cw := &ColorWorker{
		db:       db,
		srcDir:   srcDir,
		clusters: DefaultColorClusters,
	}
	for _, opt := range opts {
		opt(cw)
	}
	return cw
}

// ExtractForImage reads the image file, runs K-means to find dominant colors,
// serialises the result as a JSON array of hex strings (e.g. ["#3a1f0d",...]),
// and persists it in images.dominant_colors.
func (cw *ColorWorker) ExtractForImage(ctx context.Context, img *models.Image) error {
	if img.IsVideo {
		return nil // skip videos — trickplay handles frame extraction
	}

	srcPath := filepath.Join(cw.srcDir, img.Filename)

	pixels, err := samplePixels(srcPath, colorSampleStep)
	if err != nil {
		return fmt.Errorf("colors: sampling %q: %w", srcPath, err)
	}

	if len(pixels) == 0 {
		return fmt.Errorf("colors: no pixels sampled from %q", srcPath)
	}

	k := cw.clusters
	if k > len(pixels) {
		k = len(pixels)
	}

	centroids := kMeans(pixels, k, defaultKMeansIter)

	hexColors := make([]string, len(centroids))
	for i, c := range centroids {
		hexColors[i] = fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}

	encoded, err := json.Marshal(hexColors)
	if err != nil {
		return fmt.Errorf("colors: encoding result: %w", err)
	}

	colorsJSON := string(encoded)
	if err := cw.persist(ctx, img.ID, colorsJSON); err != nil {
		return err
	}

	slog.Info("colors: extracted", "image_id", img.ID, "colors", hexColors)
	return nil
}

// persist stores the JSON color string into images.dominant_colors.
func (cw *ColorWorker) persist(ctx context.Context, id int64, colorsJSON string) error {
	_, err := cw.db.ExecContext(ctx,
		`UPDATE images SET dominant_colors = ? WHERE id = ?`, colorsJSON, id,
	)
	if err != nil {
		return fmt.Errorf("colors: persisting for image %d: %w", id, err)
	}
	return nil
}

// samplePixels decodes the image at path and returns a sampled slice of pixels.
// step determines how many pixels to skip (larger = fewer samples, faster).
func samplePixels(path string, step int) ([]RGB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %q: %w", path, err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Pre-allocate with a capacity estimate.
	estimated := (w / step) * (h / step)
	if estimated < 1 {
		estimated = 1
	}
	pixels := make([]RGB, 0, estimated)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r32, g32, b32, a32 := img.At(x, y).RGBA()
			if a32 < 0x8000 {
				continue // skip transparent pixels
			}
			pixels = append(pixels, RGB{
				R: uint8(r32 >> 8),
				G: uint8(g32 >> 8),
				B: uint8(b32 >> 8),
			})
		}
	}

	return pixels, nil
}

// kMeans runs K-means clustering on pixels and returns the k centroids.
func kMeans(pixels []RGB, k, maxIter int) []RGB {
	// Initialise centroids by picking k random pixels.
	centroids := make([]RGB, k)
	perm := rand.Perm(len(pixels))
	for i := range k {
		centroids[i] = pixels[perm[i]]
	}

	assignments := make([]int, len(pixels))

	for range maxIter {
		// Assignment step.
		changed := false
		for i, p := range pixels {
			nearest := nearestCentroid(p, centroids)
			if assignments[i] != nearest {
				assignments[i] = nearest
				changed = true
			}
		}
		if !changed {
			break // converged
		}

		// Update step.
		sums := make([][3]float64, k)
		counts := make([]int, k)
		for i, p := range pixels {
			c := assignments[i]
			sums[c][0] += float64(p.R)
			sums[c][1] += float64(p.G)
			sums[c][2] += float64(p.B)
			counts[c]++
		}
		for j := range k {
			if counts[j] == 0 {
				continue
			}
			n := float64(counts[j])
			centroids[j] = RGB{
				R: uint8(sums[j][0] / n),
				G: uint8(sums[j][1] / n),
				B: uint8(sums[j][2] / n),
			}
		}
	}

	return centroids
}

// nearestCentroid returns the index of the centroid closest to p in RGB space.
func nearestCentroid(p RGB, centroids []RGB) int {
	best := 0
	bestDist := math.MaxFloat64
	for i, c := range centroids {
		d := colorDist(p, c)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// colorDist returns the squared Euclidean distance between two RGB colors.
func colorDist(a, b RGB) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	return dr*dr + dg*dg + db*db
}
