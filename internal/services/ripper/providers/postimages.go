package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// PostImages rips direct image URLs from postimages.org image pages.
//
// Example page: https://postimages.org/image/abc123
// The direct URL is in a meta[property="og:image"] tag or an <img id="main-image">.
type PostImages struct {
	client    *http.Client
	userAgent string
}

// postImagesOGRe tries the og:image meta tag first (most reliable).
var postImagesOGRe = regexp.MustCompile(`(?i)<meta[^>]+property="og:image"[^>]+content="(?P<url>https?://[^"]+)"`)

// postImagesImgRe falls back to the main image element.
var postImagesImgRe = regexp.MustCompile(`(?i)<img[^>]+id="main-image"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewPostImages creates a PostImages ripper.
func NewPostImages(client *http.Client, userAgent string) *PostImages {
	if client == nil {
		client = newDefaultClient()
	}
	return &PostImages{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *PostImages) Hosts() []string {
	return []string{"postimages.org", "www.postimages.org", "postimg.cc", "www.postimg.cc"}
}

// Rip implements ripper.Ripper.
func (r *PostImages) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	// Try og:image first.
	if u, err := firstMatch(postImagesOGRe, body, pageURL); err == nil {
		return []string{u}, nil
	}

	u, err := firstMatch(postImagesImgRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("postimages: %w", err)
	}

	return []string{u}, nil
}
