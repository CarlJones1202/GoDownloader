package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// ImgBox rips direct image URLs from imgbox.com image pages.
//
// The direct URL is in an <img> tag with id="img".
type ImgBox struct {
	client    *http.Client
	userAgent string
}

// imgBoxRe matches: <img id="img" ... src="https://...">
// Also handles src before id.
var imgBoxRe = regexp.MustCompile(`(?i)<img[^>]+id="img"[^>]+src="([^"]+)"`)
var imgBoxReSrcFirst = regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"[^>]+id="img"`)

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

	if m := imgBoxRe.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}
	if m := imgBoxReSrcFirst.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}

	return nil, fmt.Errorf("imgbox: no #img found on %s", pageURL)
}
