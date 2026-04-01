package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// PixHost rips direct image URLs from pixhost.to image pages.
//
// Example page: https://pixhost.to/show/123/abc.jpg
// The direct image URL is in an <img> tag with id="image".
type PixHost struct {
	client    *http.Client
	userAgent string
}

// pixHostRe matches: <img ... id="image" ... src="https://...">
var pixHostRe = regexp.MustCompile(`(?i)<img[^>]+id="image"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewPixHost creates a PixHost ripper.
func NewPixHost(client *http.Client, userAgent string) *PixHost {
	if client == nil {
		client = newDefaultClient()
	}
	return &PixHost{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *PixHost) Hosts() []string {
	return []string{"pixhost.to", "www.pixhost.to"}
}

// Rip implements ripper.Ripper.
func (r *PixHost) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(pixHostRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("pixhost: %w", err)
	}

	return []string{u}, nil
}
