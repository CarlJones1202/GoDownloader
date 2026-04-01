package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// TurboImageHost rips direct image URLs from turboimagehost.com image pages.
//
// Example page: https://www.turboimagehost.com/p/abc123/filename.jpg.html
// The direct URL is in an <img> tag with id="imageid".
type TurboImageHost struct {
	client    *http.Client
	userAgent string
}

// turboImgRe matches: <img ... id="imageid" ... src="https://...">
var turboImgRe = regexp.MustCompile(`(?i)<img[^>]+id="imageid"[^>]+src="(?P<url>https?://[^"]+)"`)

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

	u, err := firstMatch(turboImgRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("turboimagehost: %w", err)
	}

	return []string{u}, nil
}
