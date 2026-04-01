package providers

import (
	"context"
	"net/http"
	"strings"
)

// PixHost derives full-size image URLs from pixhost.to thumbnail URLs
// via a simple URL string transformation. It does NOT fetch any page.
//
// Reference: replace "/thumbs" -> "/images" and "https://t" -> "https://img".
type PixHost struct{}

// NewPixHost creates a PixHost ripper. The client and userAgent
// parameters are accepted for interface compatibility but unused.
func NewPixHost(_ *http.Client, _ string) *PixHost {
	return &PixHost{}
}

// Hosts implements ripper.Ripper.
func (r *PixHost) Hosts() []string {
	return []string{"pixhost.to", "www.pixhost.to", "t3.pixhost.to", "t4.pixhost.to", "t5.pixhost.to", "t6.pixhost.to", "t7.pixhost.to", "t8.pixhost.to", "t9.pixhost.to", "t10.pixhost.to"}
}

// Rip implements ripper.Ripper. For PixHost, if we only have the page URL
// we attempt the same transform.
func (r *PixHost) Rip(_ context.Context, pageURL string) ([]string, error) {
	return []string{transformPixHost(pageURL)}, nil
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *PixHost) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	return []string{transformPixHost(thumbnailURL)}, nil
}

// transformPixHost converts a thumbnail URL to a full-size image URL.
func transformPixHost(u string) string {
	u = strings.ReplaceAll(u, "/thumbs", "/images")
	u = strings.ReplaceAll(u, "https://t", "https://img")
	return u
}
