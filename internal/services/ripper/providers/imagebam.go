package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ImageBam rips direct image URLs from imagebam.com image pages.
//
// ImageBam shows an interstitial/NSFW consent page before serving the actual
// image. Setting both "nsfw_inter" and "sfw_inter" cookies bypasses the
// interstitial and returns the image page directly. If we still land on the
// interstitial (no img.main-image found), we detect it and re-request after
// the jar has absorbed any Set-Cookie headers.
type ImageBam struct {
	userAgent string
}

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

	// Set both NSFW and SFW interstitial cookies to bypass the consent page.
	// ImageBam checks "nsfw_inter" for NSFW content and "sfw_inter" for the
	// general interstitial/ad gate. Both must be set.
	bamURL, _ := url.Parse("https://www.imagebam.com")
	jar.SetCookies(bamURL, []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/"},
		{Name: "sfw_inter", Value: "1", Path: "/"},
	})
	// Also set on the bare domain in case the request resolves there.
	bareURL, _ := url.Parse("https://imagebam.com")
	jar.SetCookies(bareURL, []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/"},
		{Name: "sfw_inter", Value: "1", Path: "/"},
	})

	client := &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}

	// Try up to 2 times: the first request may return the interstitial page
	// which sets additional cookies; the second request should then succeed.
	for attempt := range 2 {
		imgURL, isInterstitial, err := r.fetchAndParse(ctx, client, pageURL)
		if err != nil {
			return nil, err
		}
		if imgURL != "" {
			return []string{imgURL}, nil
		}
		if !isInterstitial {
			break
		}
		// On interstitial, the jar now has any cookies the server set.
		// Brief pause before retry to avoid rate limiting.
		if attempt == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	return nil, fmt.Errorf("imagebam: no image found on %s", pageURL)
}

// fetchAndParse makes a single GET request and attempts to extract the image URL.
// Returns (imageURL, isInterstitial, error).
func (r *ImageBam) fetchAndParse(ctx context.Context, client *http.Client, pageURL string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", false, fmt.Errorf("imagebam: building request: %w", err)
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", "https://www.imagebam.com/")

	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("imagebam: GET %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("imagebam: GET %q returned %d", pageURL, resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("imagebam: parsing HTML: %w", err)
	}

	// Primary selector: img.main-image (matches AG reference).
	if src, ok := doc.Find("img.main-image").Attr("src"); ok && src != "" {
		return src, false, nil
	}

	// Secondary: look for an <img> with an "onerror" attribute inside the
	// image display container — some ImageBam pages use this pattern.
	if src, ok := doc.Find(".image-container img").Attr("src"); ok && src != "" {
		return src, false, nil
	}

	// Fallback: any <img> with a CDN or large image URL.
	var fallbackURL string
	doc.Find("img").Each(func(_ int, sel *goquery.Selection) {
		if fallbackURL != "" {
			return
		}
		src, exists := sel.Attr("src")
		if !exists || src == "" {
			return
		}
		lower := strings.ToLower(src)
		// Skip thumbnails, icons, logos, tracking pixels.
		if strings.Contains(lower, "logo") || strings.Contains(lower, "icon") ||
			strings.Contains(lower, "thumb") || strings.Contains(lower, "avatar") ||
			strings.Contains(lower, "pixel") || strings.Contains(lower, "blank") {
			return
		}
		// Accept ImageBam CDN images or images from known CDN paths.
		if strings.Contains(lower, "images") || strings.Contains(lower, "/files/") ||
			strings.Contains(lower, "bam") {
			fallbackURL = src
		}
	})
	if fallbackURL != "" {
		return fallbackURL, false, nil
	}

	// Detect the interstitial page: it contains a "Continue to your image"
	// link pointing back to the same URL.
	isInterstitial := false
	doc.Find("a").Each(func(_ int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if strings.Contains(strings.ToLower(text), "continue to your image") {
			isInterstitial = true
		}
	})

	return "", isInterstitial, nil
}
