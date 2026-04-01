package providers

import (
	"context"
	"net/http"
	"strings"
)

// ViprIm derives full-size image URLs from vipr.im thumbnail URLs
// via a simple URL string transformation. It does NOT fetch any page.
//
// Reference: replace "/th" -> "/i" in the URL path.
type ViprIm struct{}

// NewViprIm creates a ViprIm ripper. The client and userAgent
// parameters are accepted for interface compatibility but unused.
func NewViprIm(_ *http.Client, _ string) *ViprIm {
	return &ViprIm{}
}

// Hosts implements ripper.Ripper.
func (r *ViprIm) Hosts() []string {
	return []string{"vipr.im", "www.vipr.im"}
}

// Rip implements ripper.Ripper.
func (r *ViprIm) Rip(_ context.Context, pageURL string) ([]string, error) {
	return []string{transformViprIm(pageURL)}, nil
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *ViprIm) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	return []string{transformViprIm(thumbnailURL)}, nil
}

// transformViprIm converts a thumbnail URL to a full-size image URL.
func transformViprIm(u string) string {
	return strings.ReplaceAll(u, "/th", "/i")
}
