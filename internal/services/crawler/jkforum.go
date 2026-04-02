package crawler

import (
	"context"
	"regexp"
)

// JKForum parses thread pages from jkforum.net.
//
// JKForum uses a vBulletin-like layout. Posts are inside
// <div id="post_message_XXX"> tags, and image links follow the same
// pattern as ViperGirls.
type JKForum struct{}

// jkPostRe captures individual posts.
var jkPostRe = regexp.MustCompile(`(?s)<div[^>]+id="post_message_\d+"[^>]*>(.*?)</div>`)

// jkArticleRe captures Nuxt-style article containers (fallback).
var jkArticleRe = regexp.MustCompile(`(?s)<(?:article|div[^>]+class="[^"]*(?:article-content|post-content)[^"]*")[^>]*>(.*?)</(?:article|div)>`)

// jkTitleRe tries to extract a gallery title from the post.
var jkTitleRe = regexp.MustCompile(`(?i)<(?:b|strong)>([^<]{3,80})</(?:b|strong)>`)

// jkImgLinkRe captures <a href><img src> pairs.
var jkImgLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net|mymyatt\.net)[^"]*)"[^>]*>\s*<img[^>]+src="([^"]*)"`)

// jkLinkRe captures image host links (fallback, same set as ViperGirls plus mymyatt).
var jkLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="(https?://(?:www\.)?(?:imagebam\.com|imgbox\.com|imx\.to|turboimagehost\.com|vipr\.im|pixhost\.to|postimages\.org|postimg\.cc|imagetwist\.com|acidimg\.cc|mymypic\.net|mymyatt\.net)[^"]*)"`)

// NewJKForum creates a JKForum parser.
func NewJKForum() *JKForum { return &JKForum{} }

// Hosts implements SourceParser.
func (j *JKForum) Hosts() []string {
	return []string{"jkforum.net", "www.jkforum.net"}
}

// Parse implements SourceParser.
// If postID is non-empty, only that specific post is processed.
// If postID is empty, only the first post is processed.
func (j *JKForum) Parse(_ context.Context, body, _ string, postID string) (map[string][]ImageLink, error) {
	// Try vBulletin-style first.
	galleries, err := parseForumPosts(body, jkPostRe, jkTitleRe, jkImgLinkRe, jkLinkRe, postID)
	if err != nil {
		return nil, err
	}
	if len(galleries) > 0 {
		return galleries, nil
	}

	// Fall back to Nuxt-style article containers.
	return parseForumPosts(body, jkArticleRe, jkTitleRe, jkImgLinkRe, jkLinkRe, postID)
}
