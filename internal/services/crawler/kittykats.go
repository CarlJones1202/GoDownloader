package crawler

import (
	"context"
	"regexp"
)

// KittyKats parses thread pages from kitty-kats.net.
//
// KittyKats is another vBulletin-based forum. The HTML structure is very
// similar to ViperGirls and JKForum, so we reuse parseForumPosts.
type KittyKats struct{}

// kkPostRe captures individual posts.
var kkPostRe = regexp.MustCompile(`(?s)<div[^>]+id="post_message_\d+"[^>]*>(.*?)</div>`)

// kkTitleRe tries to extract a gallery title from bold/strong text.
var kkTitleRe = regexp.MustCompile(`(?i)<(?:b|strong)>([^<]{3,80})</(?:b|strong)>`)

// kkLinkRe captures image host links.
var kkLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net)[^"]*)"`)

// NewKittyKats creates a KittyKats parser.
func NewKittyKats() *KittyKats { return &KittyKats{} }

// Hosts implements SourceParser.
func (k *KittyKats) Hosts() []string {
	return []string{"kitty-kats.net", "www.kitty-kats.net"}
}

// Parse implements SourceParser.
func (k *KittyKats) Parse(_ context.Context, body, _ string) (map[string][]string, error) {
	return parseForumPosts(body, kkPostRe, kkTitleRe, kkLinkRe)
}
