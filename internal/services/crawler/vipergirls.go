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
var vgImgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"[^>]*>\s*<img[^>]+src="([^"]*)"`)

// vgLinkRe captures image host links from <a> tags (fallback, no img src).
var vgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"`)

// NewViperGirls creates a ViperGirls parser.
func NewViperGirls() *ViperGirls { return &ViperGirls{} }

// Hosts implements SourceParser.
func (v *ViperGirls) Hosts() []string {
	return []string{"vipergirls.to", "www.vipergirls.to"}
}

// Parse implements SourceParser.
func (v *ViperGirls) Parse(_ context.Context, body, _ string) (map[string][]ImageLink, error) {
	return parseForumPosts(body, vgPostRe, vgTitleRe, vgImgLinkRe, vgLinkRe)
}

// parseForumPosts is a shared parser for vBulletin-style forums.
// It extracts posts, optional titles, and image host links.
// imgLinkRe is the primary regex that captures both href and img src.
// linkRe is the fallback that captures only href.
func parseForumPosts(body string, postRe, titleRe, imgLinkRe, linkRe *regexp.Regexp) (map[string][]ImageLink, error) {
	galleries := make(map[string][]ImageLink)

	posts := postRe.FindAllStringSubmatch(body, -1)
	for i, pm := range posts {
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
			title = "Untitled " + itoa(i+1)
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
