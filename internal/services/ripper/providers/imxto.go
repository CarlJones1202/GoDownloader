package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// ImxTo rips direct image URLs from imx.to image pages.
//
// Example page: https://imx.to/i/abc123
// The direct URL appears in an <img> tag with class "centred" or "centred_resized".
type ImxTo struct {
	client    *http.Client
	userAgent string
}

// imxToRe matches the full-size image src inside an <img class="centred..."> tag.
var imxToRe = regexp.MustCompile(`(?i)<img[^>]+class="centred(?:_resized)?"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewImxTo creates an ImxTo ripper.
func NewImxTo(client *http.Client, userAgent string) *ImxTo {
	if client == nil {
		client = newDefaultClient()
	}
	return &ImxTo{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *ImxTo) Hosts() []string {
	return []string{"imx.to", "www.imx.to"}
}

// Rip implements ripper.Ripper.
func (r *ImxTo) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(imxToRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imx.to: %w", err)
	}

	return []string{u}, nil
}
