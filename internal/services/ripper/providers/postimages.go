package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PostImages scrapes direct image URLs from postimages.org / postimg.cc.
type PostImages struct {
	userAgent string
}

// NewPostImages creates a PostImages ripper.
func NewPostImages(_ *http.Client, userAgent string) *PostImages {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"
	}
	return &PostImages{userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *PostImages) Hosts() []string {
	return []string{"postimages.org", "www.postimages.org", "postimg.cc", "www.postimg.cc"}
}

// Rip implements ripper.Ripper.
func (r *PostImages) Rip(ctx context.Context, pageURL string) ([]string, error) {
	// If it already looks like a direct image URL, return it.
	if strings.Contains(pageURL, "i.postimg.cc") {
		return []string{pageURL}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("postimages: GET %q returned %d", pageURL, resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// 1. Try og:image (standard for PostImages)
	if src, ok := doc.Find("meta[property='og:image']").Attr("content"); ok && src != "" {
		return []string{src}, nil
	}

	// 2. Try the download link
	if src, ok := doc.Find("#download").Attr("href"); ok && src != "" {
		return []string{src}, nil
	}

	// 3. Fallback: any image in the main container
	if src, ok := doc.Find("img#main-image").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	return nil, fmt.Errorf("postimages: failed to extract image from %s", pageURL)
}
