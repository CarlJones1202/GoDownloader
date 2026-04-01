package crawler

import (
	"context"
	"regexp"
)

// KittyKats parses thread pages from kitty-kats.net.
//
// KittyKats is a XenForo-based forum. The HTML structure uses
// article.message--post containers. We also support the vBulletin
// fallback via parseForumPosts.
type KittyKats struct{}

// kkPostRe captures individual posts.
var kkPostRe = regexp.MustCompile(`(?s)<div[^>]+id="post_message_\d+"[^>]*>(.*?)</div>`)

// kkXFPostRe captures XenForo-style post content.
var kkXFPostRe = regexp.MustCompile(`(?s)<div[^>]+class="[^"]*bbWrapper[^"]*"[^>]*>(.*?)</div>`)

// kkTitleRe tries to extract a gallery title from bold/strong text.
var kkTitleRe = regexp.MustCompile(`(?i)<(?:b|strong)>([^<]{3,80})</(?:b|strong)>`)

// kkImgLinkRe captures <a href><img src> pairs.
var kkImgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"[^>]*>\s*<img[^>]+src="([^"]*)"`)

// kkLinkRe captures image host links (fallback).
var kkLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"`)

// NewKittyKats creates a KittyKats parser.
func NewKittyKats() *KittyKats { return &KittyKats{} }

// Hosts implements SourceParser.
func (k *KittyKats) Hosts() []string {
	return []string{"kitty-kats.net", "www.kitty-kats.net"}
}

// Parse implements SourceParser.
func (k *KittyKats) Parse(_ context.Context, body, _ string) (map[string][]ImageLink, error) {
	// Try XenForo-style bbWrapper first.
	galleries, err := parseForumPosts(body, kkXFPostRe, kkTitleRe, kkImgLinkRe, kkLinkRe)
	if err != nil {
		return nil, err
	}
	if len(galleries) > 0 {
		return galleries, nil
	}

	// Fall back to vBulletin-style.
	return parseForumPosts(body, kkPostRe, kkTitleRe, kkImgLinkRe, kkLinkRe)
}
