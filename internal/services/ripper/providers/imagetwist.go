package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// Imagetwist rips direct image URLs from imagetwist.com image pages.
//
// Example page: https://imagetwist.com/abc123/filename.jpg
// The direct URL is in an <img> tag with class "pic" inside a phun-photo div.
type Imagetwist struct {
	client    *http.Client
	userAgent string
}

// imagetwistRe matches: <img ... class="pic" src="https://...">
var imagetwistRe = regexp.MustCompile(`(?i)<img[^>]+class="pic"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewImagetwist creates an Imagetwist ripper.
func NewImagetwist(client *http.Client, userAgent string) *Imagetwist {
	if client == nil {
		client = newDefaultClient()
	}
	return &Imagetwist{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *Imagetwist) Hosts() []string {
	return []string{"imagetwist.com", "www.imagetwist.com"}
}

// Rip implements ripper.Ripper.
func (r *Imagetwist) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(imagetwistRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imagetwist: %w", err)
	}

	return []string{u}, nil
}
