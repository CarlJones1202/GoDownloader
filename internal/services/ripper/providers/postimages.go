package providers

import (
	"context"
	"net/http"
)

// PostImages is a passthrough ripper for postimages.org / postimg.cc.
// The image URLs from these hosts are typically direct links that
// need no additional scraping.
type PostImages struct{}

// NewPostImages creates a PostImages ripper.
func NewPostImages(_ *http.Client, _ string) *PostImages {
	return &PostImages{}
}

// Hosts implements ripper.Ripper.
func (r *PostImages) Hosts() []string {
	return []string{"postimages.org", "www.postimages.org", "postimg.cc", "www.postimg.cc"}
}

// Rip implements ripper.Ripper — returns the URL unchanged.
func (r *PostImages) Rip(_ context.Context, pageURL string) ([]string, error) {
	return []string{pageURL}, nil
}
