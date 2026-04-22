package crawler

import (
	"context"
	"regexp"
	"strings"
)

// ViperGirls parses thread pages from vipergirls.to.
//
// Thread structure: posts contain image host links (imagebam, imgbox, etc.)
// inside <a> tags wrapped in <img> thumbnail tags. Each post is typically
// one "gallery" worth of images.
type ViperGirls struct{}

// vgPostRe captures individual posts: <div id="post_message_XXX">...</div>
var vgPostRe = regexp.MustCompile(`(?s)<div[^>]+id="post_message_\d+"[^>]*>(.*?)</div>`)

// vgTitleRe tries to extract a gallery title from bold text or the post header.
var vgTitleRe = regexp.MustCompile(`(?i)<b>([^<]{3,80})</b>`)

// vgImgLinkRe captures <a href="..."><img src="..."></a> pairs from image host links.
// It captures both the href (group 1) and the img src (group 2).
var vgImgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.(?:to|org)|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"[^>]*>(?:[^<]*|<br[^>]*>|\s)*<img[^>]+src="([^"]*)"`)

// vgLinkRe captures image host links from <a> tags (fallback, no img src).
var vgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.(?:to|org)|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"`)

// NewViperGirls creates a ViperGirls parser.
func NewViperGirls() *ViperGirls { return &ViperGirls{} }

// Hosts implements SourceParser.
func (v *ViperGirls) Hosts() []string {
	return []string{"vipergirls.to", "www.vipergirls.to"}
}

// Parse implements SourceParser.
// If postID is non-empty, only that specific post is processed.
// If postID is empty, all posts are processed.
func (v *ViperGirls) Parse(_ context.Context, body, _ string, postID string) (map[string][]ImageLink, error) {
	return parseForumPosts(body, vgPostRe, vgTitleRe, vgImgLinkRe, vgLinkRe, postID)
}

// parseForumPosts is a shared parser for vBulletin-style forums.
// It extracts posts, optional titles, and image host links.
// If postID is non-empty, only that specific post is processed.
// If postID is empty, all posts are processed.
func parseForumPosts(body string, postRe, titleRe, imgLinkRe, linkRe *regexp.Regexp, postID string) (map[string][]ImageLink, error) {
	galleries := make(map[string][]ImageLink)

	var postRegex *regexp.Regexp
	if postID != "" {
		// Match specific post by ID
		postRegex = regexp.MustCompile(`(?s)<div[^>]+id="post_message_` + postID + `"[^>]*>(.*?)</div>`)
	} else {
		// Match all posts
		postRegex = regexp.MustCompile(`(?s)<div[^>]+id="post_message_\d+"[^>]*>(.*?)</div>`)
	}

	posts := postRegex.FindAllStringSubmatch(body, -1)

	// If filtering by specific postID, we expect at most 1 result
	// If getting all posts, process all of them
	maxPosts := len(posts)
	if postID != "" {
		maxPosts = 1 // could be 0 if post not found
	}

	for i := 0; i < len(posts) && i < maxPosts; i++ {
		pm := posts[i]
		if len(pm) < 2 {
			continue
		}
		postHTML := pm[1]

		// Try to get a title from bold text in the post.
		title := ""
		if tm := titleRe.FindStringSubmatch(postHTML); tm != nil {
			title = strings.TrimSpace(tm[1])
		}
		if title == "" {
			title = "Untitled"
		}

		// First pass: try to extract <a href><img src> pairs.
		seen := make(map[string]bool)
		imgLinks := imgLinkRe.FindAllStringSubmatch(postHTML, -1)
		for _, m := range imgLinks {
			if len(m) < 3 {
				continue
			}
			href := strings.TrimSpace(m[1])
			if seen[href] {
				continue
			}
			seen[href] = true

			thumbURL := strings.TrimSpace(m[2])

			// If the href is already a gallery URL or a direct image link,
			// we prefer to scrape the page or download the direct link
			// rather than relying on a potentially broken thumbnail transform.
			if isGalleryURL(href) || isDirectImageURL(href) {
				thumbURL = ""
			}

			galleries[title] = append(galleries[title], ImageLink{
				PageURL:  href,
				ThumbURL: thumbURL,
			})
		}

		// Second pass: find any <a> links not already captured (no img child).
		links := linkRe.FindAllStringSubmatch(postHTML, -1)
		for _, lm := range links {
			if len(lm) < 2 {
				continue
			}
			href := strings.TrimSpace(lm[1])
			if seen[href] {
				continue
			}
			seen[href] = true
			galleries[title] = append(galleries[title], ImageLink{
				PageURL: href,
			})
		}
	}

	return galleries, nil
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// isGalleryURL returns true if the URL looks like a gallery/album page.
func isGalleryURL(u string) bool {
	u = strings.ToLower(u)
	return strings.Contains(u, "/gallery/") ||
		strings.Contains(u, "/album/") ||
		strings.Contains(u, "/view/g") || // ImageBam galleries
		strings.Contains(u, "/p/") // Some hosts use /p/ for albums
}

// isDirectImageURL returns true if the URL looks like a direct link to an image.
func isDirectImageURL(u string) bool {
	u = strings.ToLower(u)

	// Explicitly exclude common viewer page patterns
	if strings.Contains(u, "/show/") || // Pixhost viewer
		strings.Contains(u, "/p/") || // TurboImageHost / Imagetwist viewer
		strings.Contains(u, "/view/") ||
		strings.Contains(u, "img.php") {
		return false
	}

	// Common direct link indicators
	if strings.Contains(u, "/images/") ||
		strings.Contains(u, "/i/") ||
		strings.Contains(u, "/img/") {
		return true
	}

	// Check for image extensions as a fallback
	exts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}
	for _, ext := range exts {
		if strings.HasSuffix(u, ext) {
			// Double check it's not a known viewer host that uses .jpg for viewer pages
			if strings.Contains(u, "pixhost.") || strings.Contains(u, "turboimagehost.") {
				// For these hosts, we already checked for viewer patterns above.
				// If we reached here, it might be a direct link if it also has /images/ or /i/.
				return false // Safest fallback for these hosts
			}
			return true
		}
	}
	return false
}
