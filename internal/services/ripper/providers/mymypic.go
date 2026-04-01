package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// MyMyPic rips direct image URLs from mymypic.net image pages.
//
// Example page: https://mymypic.net/img-abc123.html
// The direct URL is in an og:image meta tag.
type MyMyPic struct {
	client    *http.Client
	userAgent string
}

// myMyPicRe matches: <meta property="og:image" content="https://...">
var myMyPicRe = regexp.MustCompile(`(?i)<meta[^>]+property="og:image"[^>]+content="(?P<url>https?://[^"]+)"`)

// myMyPicImgRe is a fallback matching an <img> with id="imgprev" or class="pic".
var myMyPicImgRe = regexp.MustCompile(`(?i)<img[^>]+(?:id="imgprev"|class="pic")[^>]+src="(?P<url>https?://[^"]+)"`)

// NewMyMyPic creates a MyMyPic ripper.
func NewMyMyPic(client *http.Client, userAgent string) *MyMyPic {
	if client == nil {
		client = newDefaultClient()
	}
	return &MyMyPic{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *MyMyPic) Hosts() []string {
	return []string{"mymypic.net", "www.mymypic.net"}
}

// Rip implements ripper.Ripper.
func (r *MyMyPic) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	if u, err := firstMatch(myMyPicRe, body, pageURL); err == nil {
		return []string{u}, nil
	}

	u, err := firstMatch(myMyPicImgRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("mymypic: %w", err)
	}

	return []string{u}, nil
}
