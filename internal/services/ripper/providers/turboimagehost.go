package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// TurboImageHost rips direct image URLs from turboimagehost.com image pages.
//
// The direct URL is in an <img> with id="imageid", or fallback
// selectors #img and #uImageCont img.
type TurboImageHost struct {
	client    *http.Client
	userAgent string
}

// turboImgIDRe matches: <img ... id="imageid" ... src="...">
var turboImgIDRe = regexp.MustCompile(`(?i)<img[^>]+id="imageid"[^>]+src="([^"]+)"`)

// turboImgIDSrcFirst matches: <img src="..." ... id="imageid">
var turboImgIDSrcFirst = regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"[^>]+id="imageid"`)

// turboImgRe matches: <img id="img" ... src="...">
var turboImgTagRe = regexp.MustCompile(`(?i)<img[^>]+id="img"[^>]+src="([^"]+)"`)

// turboUImageRe matches images inside #uImageCont.
var turboUImageRe = regexp.MustCompile(`(?i)id="uImageCont"[^>]*>[\s\S]*?<img[^>]+src="([^"]+)"`)

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

	// Try primary selector: img#imageid
	if m := turboImgIDRe.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}
	if m := turboImgIDSrcFirst.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}
	// Fallback: img#img
	if m := turboImgTagRe.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}
	// Fallback: #uImageCont img
	if m := turboUImageRe.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}

	return nil, fmt.Errorf("turboimagehost: no image found on %s", pageURL)
}
