package providers

import (
	"context"
	"net/http"
	"strings"
)

// MyMyPic is a passthrough ripper for mymypic.net / mymyatt.net.
// Links from JKForum are typically direct image URLs that may need
// a simple protocol prefix fix.
type MyMyPic struct{}

// NewMyMyPic creates a MyMyPic ripper.
func NewMyMyPic(_ *http.Client, _ string) *MyMyPic {
	return &MyMyPic{}
}

// Hosts implements ripper.Ripper.
func (r *MyMyPic) Hosts() []string {
	return []string{"mymypic.net", "www.mymypic.net", "mymyatt.net", "www.mymyatt.net"}
}

// Rip implements ripper.Ripper.
func (r *MyMyPic) Rip(_ context.Context, pageURL string) ([]string, error) {
	u := pageURL
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	return []string{u}, nil
}

// RipThumbnail implements ripper.ThumbnailRipper.
// For MyMyPic, the thumbnail URL from the forum post may itself be the
// full-size image, so we try both.
func (r *MyMyPic) RipThumbnail(_ context.Context, thumbnailURL string) ([]string, error) {
	u := thumbnailURL
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	return []string{u}, nil
}
