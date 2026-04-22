package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// TurboImageHost rips direct image URLs from turboimagehost.com image pages.
//
// The direct URL is in an <img> with id="img" (primary, matching AG reference),
// or fallback #uImageCont img.
type TurboImageHost struct {
	client    *http.Client
	userAgent string
}

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
	// If it's a gallery/album page
	if strings.Contains(pageURL, "/album/") {
		return r.ripGallery(ctx, pageURL)
	}

	body, err := fetchPage(ctx, r.client, pageURL, r.userAgent)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("turboimagehost: parsing HTML: %w", err)
	}

	// Primary selector: #img — matches the AG reference exactly.
	if src, ok := doc.Find("#img").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	// Fallback: #uImageCont img — matches the AG reference fallback.
	if src, ok := doc.Find("#uImageCont img").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	// Additional fallback: img#imageid (some TurboImageHost variants use this).
	if src, ok := doc.Find("img#imageid").Attr("src"); ok && src != "" {
		return []string{src}, nil
	}

	// If no ID-based image found, try searching for the first image that looks like the main one.
	// Usually it's in a specific container.
	var fallbackSrc string
	doc.Find("img.img-responsive, .image-container img").Each(func(_ int, s *goquery.Selection) {
		if src, ok := s.Attr("src"); ok && fallbackSrc == "" {
			if strings.Contains(src, "/i/") || strings.Contains(src, "/img/") {
				fallbackSrc = src
			}
		}
	})
	if fallbackSrc != "" {
		return []string{fallbackSrc}, nil
	}

	return nil, fmt.Errorf("turboimagehost: no image found on %s", pageURL)
}

func (r *TurboImageHost) ripGallery(ctx context.Context, pageURL string) ([]string, error) {
	var allLinks []string
	page := 1

	for {
		url := pageURL
		if page > 1 {
			// TurboImageHost pagination usually adds ?p=N or similar
			if strings.Contains(url, "?") {
				url = fmt.Sprintf("%s&p=%d", url, page)
			} else {
				url = fmt.Sprintf("%s?p=%d", url, page)
			}
		}

		body, err := fetchPage(ctx, r.client, url, r.userAgent)
		if err != nil {
			break // or return error if first page fails
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			break
		}

		found := 0
		// Thumbnails in galleries are usually linked to image pages
		doc.Find(".thumb a, .album-images a, .image-container a").Each(func(_ int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				if strings.Contains(href, "/p/") {
					allLinks = append(allLinks, href)
					found++
				}
			}
		})

		// If no more images found or it's the last page
		if found == 0 {
			break
		}

		// Simple safeguard against infinite loops
		if page > 50 {
			break
		}
		page++
	}

	if len(allLinks) == 0 {
		return nil, fmt.Errorf("turboimagehost: no images found in album %s", pageURL)
	}

	return allLinks, nil
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *TurboImageHost) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	// TurboImageHost thumbnail URLs look like:
	// https://sbd053.turboimagehost.com/t/121631840/MetArt_Soft-Curls_Kira-Rami_high_0085.jpg
	// Direct URLs look like:
	// https://sbd053.turboimagehost.com/i/121631840/MetArt_Soft-Curls_Kira-Rami_high_0085.jpg
	return []string{strings.ReplaceAll(thumbnailURL, "/t/", "/i/")}, nil
}
