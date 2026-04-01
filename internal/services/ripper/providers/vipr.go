package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// ViprIm rips direct image URLs from vipr.im image pages.
//
// Example page: https://vipr.im/abc123
// The direct URL appears in an <img> tag inside a div with class "image-viewer".
type ViprIm struct {
	client    *http.Client
	userAgent string
}

// viprImRe matches the main image src inside the viewer container.
var viprImRe = regexp.MustCompile(`(?i)<div[^>]+class="[^"]*image-viewer[^"]*"[^>]*>[\s\S]*?<img[^>]+src="(?P<url>https?://[^"]+)"`)

// NewViprIm creates a ViprIm ripper.
func NewViprIm(client *http.Client, userAgent string) *ViprIm {
	if client == nil {
		client = newDefaultClient()
	}
	return &ViprIm{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *ViprIm) Hosts() []string {
	return []string{"vipr.im", "www.vipr.im"}
}

// Rip implements ripper.Ripper.
func (r *ViprIm) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(viprImRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("vipr.im: %w", err)
	}

	return []string{u}, nil
}
