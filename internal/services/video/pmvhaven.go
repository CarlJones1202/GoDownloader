package video

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// PMVHavenRipper extracts video URLs from PMVHaven pages.
type PMVHavenRipper struct {
	client    *http.Client
	userAgent string
}

// NewPMVHavenRipper creates a PMVHavenRipper.
func NewPMVHavenRipper(client *http.Client, userAgent string) *PMVHavenRipper {
	return &PMVHavenRipper{client: client, userAgent: userAgent}
}

func (r *PMVHavenRipper) Hosts() []string {
	return []string{"pmvhaven.com", "www.pmvhaven.com"}
}

func (r *PMVHavenRipper) Rip(ctx context.Context, pageURL string) (*RipResult, error) {
	slog.Debug("pmvhaven: ripping", "url", pageURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pmvhaven: creating request: %w", err)
	}
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pmvhaven: fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pmvhaven: page returned %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("pmvhaven: parsing HTML: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1").First().Text())
	title = strings.TrimSuffix(title, " - PMVHaven")
	if title == "" {
		title = "Unknown Video"
	}

	var videoURL string

	// Method 1: Search script tags for .mp4 URLs (prefer pmvhaven.com hosted).
	mp4Regex := regexp.MustCompile(`https?://[^"'\s<>]+\.mp4`)
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		if videoURL != "" {
			return
		}
		text := s.Text()
		matches := mp4Regex.FindAllString(text, -1)
		for _, match := range matches {
			if strings.Contains(match, "pmvhaven.com") {
				videoURL = match
				slog.Debug("pmvhaven: found in script", "url", videoURL)
				return
			}
		}
		// Accept any .mp4 if no pmvhaven-specific one found.
		if videoURL == "" && len(matches) > 0 {
			videoURL = matches[0]
		}
	})

	// Method 2: og:video meta tag.
	if videoURL == "" {
		if content, ok := doc.Find("meta[property='og:video']").Attr("content"); ok && strings.HasSuffix(content, ".mp4") {
			videoURL = content
		}
	}
	if videoURL == "" {
		if content, ok := doc.Find("meta[property='og:video:url']").Attr("content"); ok && strings.HasSuffix(content, ".mp4") {
			videoURL = content
		}
	}

	// Method 3: HTML5 video source tags.
	if videoURL == "" {
		doc.Find("video source").Each(func(_ int, s *goquery.Selection) {
			if videoURL != "" {
				return
			}
			if src, exists := s.Attr("src"); exists && strings.Contains(src, ".mp4") {
				videoURL = src
			}
		})
	}

	if videoURL == "" {
		return nil, fmt.Errorf("pmvhaven: no video URL found on %s", pageURL)
	}

	return &RipResult{DirectURL: videoURL, Title: title}, nil
}
