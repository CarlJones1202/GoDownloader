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

// TnaFlixRipper extracts video URLs from TnaFlix pages.
type TnaFlixRipper struct {
	client    *http.Client
	userAgent string
}

// NewTnaFlixRipper creates a TnaFlixRipper.
func NewTnaFlixRipper(client *http.Client, userAgent string) *TnaFlixRipper {
	return &TnaFlixRipper{client: client, userAgent: userAgent}
}

func (r *TnaFlixRipper) Hosts() []string {
	return []string{"tnaflix.com", "www.tnaflix.com"}
}

func (r *TnaFlixRipper) Rip(ctx context.Context, pageURL string) (*RipResult, error) {
	slog.Debug("tnaflix: ripping", "url", pageURL)

	videoIDRegex := regexp.MustCompile(`video(\d+)`)
	matches := videoIDRegex.FindStringSubmatch(pageURL)
	videoID := ""
	if len(matches) >= 2 {
		videoID = matches[1]
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tnaflix: creating request: %w", err)
	}
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tnaflix: fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tnaflix: page returned %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tnaflix: parsing HTML: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1").First().Text())
	title = strings.TrimSuffix(title, " - TnaFlix")
	if title == "" {
		title = "Unknown Video"
		if videoID != "" {
			title += " " + videoID
		}
	}

	var videoURL string

	// Method 1: HTML5 video source tags — pick highest quality.
	type candidate struct {
		url     string
		quality int
	}
	var candidates []candidate

	doc.Find("video source").Each(func(_ int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || !strings.Contains(src, ".mp4") {
			return
		}

		quality := 0
		if sizeStr, ok := s.Attr("size"); ok {
			fmt.Sscanf(sizeStr, "%d", &quality)
		}
		if quality == 0 {
			re := regexp.MustCompile(`(\d{3,4})p`)
			if m := re.FindStringSubmatch(src); len(m) > 1 {
				fmt.Sscanf(m[1], "%d", &quality)
			}
		}

		candidates = append(candidates, candidate{url: src, quality: quality})
	})

	if len(candidates) > 0 {
		best := candidates[0]
		for _, c := range candidates {
			if c.quality > best.quality {
				best = c
			}
		}
		videoURL = best.url
		slog.Debug("tnaflix: found HTML5 source", "quality", best.quality, "url", videoURL)
	}

	// Method 2: JavaScript variables.
	if videoURL == "" {
		doc.Find("script").Each(func(_ int, s *goquery.Selection) {
			if videoURL != "" {
				return
			}
			text := s.Text()
			patterns := []string{
				`video_url["\s:=]+["']([^"']+\.mp4[^"']*)["']`,
				`file["\s:=]+["']([^"']+\.mp4[^"']*)["']`,
				`src["\s:=]+["']([^"']+\.mp4[^"']*)["']`,
			}
			for _, pat := range patterns {
				re := regexp.MustCompile(pat)
				if m := re.FindStringSubmatch(text); len(m) > 1 && strings.Contains(m[1], ".mp4") {
					videoURL = m[1]
					return
				}
			}
		})
	}

	// Method 3: CDN fallback.
	if videoURL == "" && videoID != "" {
		fallbackURL := fmt.Sprintf("https://static.tnaflix.com/contents/videos_sources/%s/file.mp4", videoID)
		headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, fallbackURL, nil)
		if err == nil {
			headReq.Header.Set("Referer", pageURL)
			headReq.Header.Set("User-Agent", r.userAgent)
			if headResp, err := r.client.Do(headReq); err == nil {
				headResp.Body.Close()
				if headResp.StatusCode == http.StatusOK {
					videoURL = fallbackURL
					slog.Debug("tnaflix: CDN fallback", "url", videoURL)
				}
			}
		}
	}

	if videoURL == "" {
		return nil, fmt.Errorf("tnaflix: no video URL found on %s", pageURL)
	}

	return &RipResult{DirectURL: videoURL, Title: title}, nil
}
