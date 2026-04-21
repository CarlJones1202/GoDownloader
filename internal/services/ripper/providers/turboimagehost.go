package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// TurboImageHost rips direct image URLs from turboimagehost.com image pages.
//
// The direct URL is in an <img> with id="img" (primary, matching AG reference),
// or fallback #uImageCont img.
type TurboImageHost struct {
	client    *http.Client
	userAgent string
}

// NewTurboImageHost creates a TurboImageHost ripper.
func NewTurboImageHost(client *http.Client, userAgent string) *TurboImageHost {
	if client == nil {
		client = newDefaultClient()
	}
	return &TurboImageHost{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *TurboImageHost) Hosts() []string {
	return []string{"turboimagehost.com", "www.turboimagehost.com"}
}

// Rip implements ripper.Ripper.
func (r *TurboImageHost) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("turboimagehost: parsing HTML: %w", err)
	}

	// Primary selector: #img — matches the AG reference exactly.
	if src, ok := doc.Find("#img").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	// Fallback: #uImageCont img — matches the AG reference fallback.
	if src, ok := doc.Find("#uImageCont img").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	// Additional fallback: img#imageid (some TurboImageHost variants use this).
	if src, ok := doc.Find("img#imageid").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	return nil, fmt.Errorf("turboimagehost: no image found on %s", pageURL)
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *TurboImageHost) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	// TurboImageHost thumbnail URLs look like:
	// https://sbd053.turboimagehost.com/t/121631840/MetArt_Soft-Curls_Kira-Rami_high_0085.jpg
	// Direct URLs look like:
	// https://sbd053.turboimagehost.com/i/121631840/MetArt_Soft-Curls_Kira-Rami_high_0085.jpg
	return []string{strings.ReplaceAll(thumbnailURL, "/t/", "/i/")}, nil
}
