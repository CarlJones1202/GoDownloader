package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// ImageBam rips direct image URLs from imagebam.com image pages.
//
// Example page: https://www.imagebam.com/image/abc123
// The direct image URL is in an <img> tag with class "main-image".
type ImageBam struct {
	client    *http.Client
	userAgent string
}

// imageBAMRe matches:  <img ... class="main-image" src="https://...">
var imageBAMRe = regexp.MustCompile(`(?i)<img[^>]+class="main-image"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewImageBam creates an ImageBam ripper.
func NewImageBam(client *http.Client, userAgent string) *ImageBam {
	if client == nil {
		client = newDefaultClient()
	}
	return &ImageBam{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *ImageBam) Hosts() []string {
	return []string{"imagebam.com", "www.imagebam.com"}
}

// Rip implements ripper.Ripper.
func (r *ImageBam) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(imageBAMRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imagebam: %w", err)
	}

	return []string{u}, nil
}
