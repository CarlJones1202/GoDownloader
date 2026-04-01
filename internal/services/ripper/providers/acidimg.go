package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// AcidImg rips direct image URLs from acidimg.cc image pages.
//
// Example page: https://acidimg.cc/img-abc123.html
// The direct URL is in an <img> tag with class "centred" or "centred_resized".
// (AcidImg uses the same template engine as Imx.to.)
type AcidImg struct {
	client    *http.Client
	userAgent string
}

// acidImgRe matches the full-size image inside a centred class img tag.
var acidImgRe = regexp.MustCompile(`(?i)<img[^>]+class="centred(?:_resized)?"[^>]+src="(?P<url>https?://[^"]+)"`)

// NewAcidImg creates an AcidImg ripper.
func NewAcidImg(client *http.Client, userAgent string) *AcidImg {
	if client == nil {
		client = newDefaultClient()
	}
	return &AcidImg{client: client, userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *AcidImg) Hosts() []string {
	return []string{"acidimg.cc", "www.acidimg.cc"}
}

// Rip implements ripper.Ripper.
func (r *AcidImg) Rip(ctx context.Context, pageURL string) ([]string, error) {
	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	u, err := firstMatch(acidImgRe, body, pageURL)
	if err != nil {
		return nil, fmt.Errorf("acidimg: %w", err)
	}

	return []string{u}, nil
}
