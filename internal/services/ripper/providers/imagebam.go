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
	// Handle galleries
	if strings.Contains(pageURL, "/gallery/") || strings.Contains(pageURL, "/view/G") {
		return r.ripGallery(ctx, pageURL)
	}

	jar, _ := cookiejar.New(nil)

	// Set both NSFW and SFW interstitial cookies to bypass the consent page.
	bamURL, _ := url.Parse("https://www.imagebam.com")
	jar.SetCookies(bamURL, []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/"},
		{Name: "sfw_inter", Value: "1", Path: "/"},
	})
	bareURL, _ := url.Parse("https://imagebam.com")
	jar.SetCookies(bareURL, []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/"},
		{Name: "sfw_inter", Value: "1", Path: "/"},
	})

	client := &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}

	// Try up to 2 times to handle the interstitial correctly.
	for attempt := range 2 {
		imgURLs, isInterstitial, err := r.fetchAndParse(ctx, client, pageURL)
		if err != nil {
			return nil, err
		}
		if len(imgURLs) > 0 {
			return imgURLs, nil
		}
		if !isInterstitial {
			break
		}
		// On interstitial, the jar now has any cookies the server set.
		if attempt == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("imagebam: no image found on %s", pageURL)
}

func (r *ImageBam) ripGallery(ctx context.Context, pageURL string) ([]string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 60 * time.Second, Jar: jar}
	
	// Pre-set cookies for gallery view too
	bamURL, _ := url.Parse("https://www.imagebam.com")
	jar.SetCookies(bamURL, []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/"},
		{Name: "sfw_inter", Value: "1", Path: "/"},
	})

	var allLinks []string
	nextURL := pageURL

	for nextURL != "" {
		body, err := fetchPage(ctx, client, nextURL, r.userAgent)
		if err != nil {
			break
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			break
		}

		found := 0
		// Gallery items are usually links starting with /view/ or /image/
		doc.Find("a").Each(func(_ int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				// Avoid recursion back to gallery or current page
				if (strings.Contains(href, "/view/") || strings.Contains(href, "/image/")) && 
				   !strings.Contains(href, "/view/G") && !strings.Contains(href, "/gallery/") {
					// Ensure absolute URL
					if strings.HasPrefix(href, "/") {
						href = "https://www.imagebam.com" + href
					}
					allLinks = append(allLinks, href)
					found++
				}
			}
		})

		if found == 0 {
			break
		}

		// Look for next page
		nextURL = ""
		doc.Find("a[rel='next'], a[aria-label='Next']").Each(func(_ int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				if strings.HasPrefix(href, "/") {
					nextURL = "https://www.imagebam.com" + href
				} else {
					nextURL = href
				}
			}
		})
		
		if nextURL != "" {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Deduplicate links
	uniqueLinks := make([]string, 0, len(allLinks))
	seen := make(map[string]bool)
	for _, l := range allLinks {
		if !seen[l] {
			seen[l] = true
			uniqueLinks = append(uniqueLinks, l)
		}
	}

	if len(uniqueLinks) == 0 {
		return nil, fmt.Errorf("imagebam: no images found in gallery %s", pageURL)
	}

	return uniqueLinks, nil
}

// fetchAndParse makes a single GET request and attempts to extract the image URL.
// Returns (imageURLs, isInterstitial, error).
func (r *ImageBam) fetchAndParse(ctx context.Context, client *http.Client, pageURL string) ([]string, bool, error) {
	body, err := fetchPage(ctx, client, pageURL, r.userAgent)
	if err != nil {
		return nil, false, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("imagebam: parsing HTML: %w", err)
	}

	// Primary selector: img.main-image or img.img-responsive
	var foundURL string
	doc.Find("img.main-image, img.img-responsive, .image-container img").Each(func(_ int, s *goquery.Selection) {
		if src, ok := s.Attr("src"); ok && src != "" {
			if strings.Contains(src, "imagebam.com") || strings.Contains(src, "images") {
				foundURL = src
			}
		}
	})
	
	if foundURL != "" {
		return []string{foundURL}, false, nil
	}

	// Detect the interstitial page: it contains a "Continue to your image"
	isInterstitial := false
	doc.Find("a").Each(func(_ int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if strings.Contains(strings.ToLower(text), "continue to your image") {
			isInterstitial = true
		}
	})

	return nil, isInterstitial, nil
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *ImageBam) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	// ImageBam thumbnail URLs look like:
	// https://thumbs2.imagebam.com/53/98/12/123456789.jpg
	// Direct URLs look like:
	// https://images2.imagebam.com/53/98/12/123456789.jpg
	res := strings.ReplaceAll(thumbnailURL, "thumbs", "images")
	// Some variants might use /th/ vs /images/
	res = strings.ReplaceAll(res, "/th/", "/images/")
	return []string{res}, nil
}
