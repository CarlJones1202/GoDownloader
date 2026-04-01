package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"time"
)

// ImageBam rips direct image URLs from imagebam.com image pages.
//
// ImageBam requires NSFW cookies to be set before the page will serve
// the full-size image. The image is in an <img class="main-image"> tag.
type ImageBam struct {
	userAgent string
}

// imageBamRe matches: <img ... class="main-image" src="https://...">
// Also handles src before class.
var imageBamRe = regexp.MustCompile(`(?i)<img[^>]+class="main-image"[^>]+src="([^"]+)"`)
var imageBamReSrcFirst = regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"[^>]+class="main-image"`)

// NewImageBam creates an ImageBam ripper.
func NewImageBam(_ *http.Client, userAgent string) *ImageBam {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"
	}
	return &ImageBam{userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *ImageBam) Hosts() []string {
	return []string{"imagebam.com", "www.imagebam.com"}
}

// Rip implements ripper.Ripper.
func (r *ImageBam) Rip(ctx context.Context, pageURL string) ([]string, error) {
	jar, _ := cookiejar.New(nil)

	// Set NSFW cookies so imagebam serves the full image page.
	targetURL, _ := url.Parse("https://www.imagebam.com")
	jar.SetCookies(targetURL, []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/", Domain: "imagebam.com"},
		{Name: "expires", Value: time.Now().AddDate(0, 0, 1).Format(time.RFC1123), Path: "/", Domain: "imagebam.com"},
	})

	client := &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imagebam: building request: %w", err)
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imagebam: GET %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagebam: GET %q returned %d", pageURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imagebam: reading body: %w", err)
	}
	body := string(data)

	// Try class-first pattern.
	if m := imageBamRe.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}
	// Try src-first pattern.
	if m := imageBamReSrcFirst.FindStringSubmatch(body); m != nil {
		return []string{m[1]}, nil
	}

	return nil, fmt.Errorf("imagebam: no main-image found on %s", pageURL)
}
