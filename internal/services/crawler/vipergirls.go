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

// vgLinkRe captures image host links from <a> tags.
var vgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"`)

// NewViperGirls creates a ViperGirls parser.
func NewViperGirls() *ViperGirls { return &ViperGirls{} }

// Hosts implements SourceParser.
func (v *ViperGirls) Hosts() []string {
	return []string{"vipergirls.to", "www.vipergirls.to"}
}

// Parse implements SourceParser.
func (v *ViperGirls) Parse(_ context.Context, body, _ string) (map[string][]string, error) {
	return parseForumPosts(body, vgPostRe, vgTitleRe, vgLinkRe)
}

// parseForumPosts is a shared parser for vBulletin-style forums.
// It extracts posts, optional titles, and image host links.
func parseForumPosts(body string, postRe, titleRe, linkRe *regexp.Regexp) (map[string][]string, error) {
	galleries := make(map[string][]string)

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

		links := linkRe.FindAllStringSubmatch(postHTML, -1)
		if len(links) == 0 {
			continue
		}

		seen := make(map[string]bool)
		for _, lm := range links {
			if len(lm) < 2 {
				continue
			}
			href := strings.TrimSpace(lm[1])
			if seen[href] {
				continue
			}
			seen[href] = true
			galleries[title] = append(galleries[title], href)
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
