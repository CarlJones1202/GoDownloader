package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PixHost derives full-size image URLs from pixhost.to thumbnail URLs
// via simple URL string transformations, and scrapes image/gallery pages.
type PixHost struct {
	client    *http.Client
	userAgent string
}

// NewPixHost creates a PixHost ripper.
func NewPixHost(client *http.Client, userAgent string) *PixHost {
	if client == nil {
		client = newDefaultClient()
	}
	return &PixHost{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *PixHost) Hosts() []string {
	hosts := []string{"pixhost.to", "www.pixhost.to", "pixhost.org", "www.pixhost.org"}
	for i := 1; i <= 20; i++ {
		hosts = append(hosts, fmt.Sprintf("t%d.pixhost.to", i))
		hosts = append(hosts, fmt.Sprintf("t%d.pixhost.org", i))
		hosts = append(hosts, fmt.Sprintf("img%d.pixhost.to", i))
		hosts = append(hosts, fmt.Sprintf("img%d.pixhost.org", i))
	}
	return hosts
}

// Rip implements ripper.Ripper.
func (r *PixHost) Rip(ctx context.Context, pageURL string) ([]string, error) {
	// If it's already a direct image link, just return it (maybe transformed).
	if isDirectImageURL(pageURL) {
		return []string{transformPixHost(pageURL)}, nil
	}

	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pixhost: parsing HTML: %w", err)
	}

	// Case 1: Gallery page
	if strings.Contains(pageURL, "/gallery/") {
		var links []string
		// Standard gallery: <div class="images"> ... <a href="...">
		doc.Find(".images a, .gallery-images a").Each(func(_ int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				links = append(links, href)
			}
		})
		if len(links) > 0 {
			return links, nil
		}
	}

	// Case 2: Single image viewer page (/show/)
	// Selector: class="image-img" (from imagehosts.py)
	if src, ok := doc.Find(".image-img, img#image").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	// Fallback to transform if nothing found but looks like it might work
	if strings.Contains(pageURL, "/thumbs/") {
		return []string{transformPixHost(pageURL)}, nil
	}

	return nil, fmt.Errorf("pixhost: no image found on %s", pageURL)
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *PixHost) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	return []string{transformPixHost(thumbnailURL)}, nil
}

// transformPixHost converts a thumbnail URL to a full-size image URL.
func transformPixHost(u string) string {
	// Pixhost usually uses /thumbs/ -> /images/ and tX -> imgX
	res := strings.ReplaceAll(u, "/thumbs/", "/images/")
	
	// Handle subdomain replacement: tX -> imgX
	re := regexp.MustCompile(`https?://t(\d+)\.pixhost\.(to|org)`)
	res = re.ReplaceAllString(res, "https://img$1.pixhost.$2")

	// If it's on www but has /thumbs/, it might need to stay on www or move to imgX.
	// Historically, simple replace was often enough if it was already on imgX.
	
	return res
}

// isDirectImageURL is duplicated here or moved to a shared place.
// Since providers is one package, I can just use it if I put it in helpers.go or here.
func isDirectImageURL(u string) bool {
	u = strings.ToLower(u)
	return strings.HasSuffix(u, ".jpg") || strings.HasSuffix(u, ".jpeg") ||
		strings.HasSuffix(u, ".png") || strings.HasSuffix(u, ".gif") ||
		strings.HasSuffix(u, ".webp")
}
