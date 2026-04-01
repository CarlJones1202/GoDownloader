package providers

import (
	"net/http"

	"github.com/carlj/godownload/internal/services/ripper"
)

// RegisterAll registers every built-in image provider ripper into reg.
// Pass a shared *http.Client (or nil to use per-provider defaults) and
// a user agent string.
func RegisterAll(reg *ripper.Registry, client *http.Client, userAgent string) {
	reg.Register(NewImageBam(client, userAgent))
	reg.Register(NewImgBox(client, userAgent))
	reg.Register(NewImxTo(client, userAgent))
	reg.Register(NewTurboImageHost(client, userAgent))
	reg.Register(NewViprIm(client, userAgent))
	reg.Register(NewPixHost(client, userAgent))
	reg.Register(NewPostImages(client, userAgent))
	reg.Register(NewImagetwist(client, userAgent))
	reg.Register(NewAcidImg(client, userAgent))
	reg.Register(NewMyMyPic(client, userAgent))
}
