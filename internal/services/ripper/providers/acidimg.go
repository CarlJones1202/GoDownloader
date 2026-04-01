package providers

import (
	"context"
	"net/http"
	"strings"
)

// AcidImg derives full-size image URLs from acidimg.cc thumbnail URLs
// via a simple URL string transformation. It does NOT fetch any page.
//
// Reference: the thumbnail URL has "t." in the hostname and "/t" in the
// path. Replace "t." -> "i." and "/t" -> "/i" to get the full image.
type AcidImg struct{}

// NewAcidImg creates an AcidImg ripper. The client and userAgent
// parameters are accepted for interface compatibility but unused.
func NewAcidImg(_ *http.Client, _ string) *AcidImg {
	return &AcidImg{}
}

// Hosts implements ripper.Ripper.
func (r *AcidImg) Hosts() []string {
	return []string{"acidimg.cc", "www.acidimg.cc"}
}

// Rip implements ripper.Ripper. For AcidImg, if we only have the page URL
// (no thumbnail), we attempt the same transform which may work if the
// page URL itself contains the thumbnail-style path.
func (r *AcidImg) Rip(_ context.Context, pageURL string) ([]string, error) {
	return []string{transformAcidImg(pageURL)}, nil
}

// RipThumbnail implements ripper.ThumbnailRipper.
func (r *AcidImg) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	return []string{transformAcidImg(thumbnailURL)}, nil
}

// transformAcidImg converts a thumbnail URL to a full-size image URL.
func transformAcidImg(u string) string {
	u = strings.ReplaceAll(u, "t.", "i.")
	u = strings.ReplaceAll(u, "/t", "/i")
	return u
}
