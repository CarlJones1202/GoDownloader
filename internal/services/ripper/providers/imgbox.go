package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// ImgBox rips direct image URLs from imgbox.com image pages.
//
// Example page: https://imgbox.com/abc123
// The direct URL is in an <img> tag with id="img".
type ImgBox struct {
	client    *http.Client
	userAgent string
}

// imgBoxRe matches: <img id="img" ... src="https://...">
var imgBoxRe = regexp.MustCompile(`(?i)<img[^>]+id="img"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewImgBox creates an ImgBox ripper.
func NewImgBox(client *http.Client, userAgent string) *ImgBox {
	if client == nil {
		client = newDefaultClient()
	}
	return &ImgBox{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *ImgBox) Hosts() []string {
	return []string{"imgbox.com", "www.imgbox.com"}
}

// Rip implements ripper.Ripper.
func (r *ImgBox) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(imgBoxRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imgbox: %w", err)
	}

	return []string{u}, nil
}
