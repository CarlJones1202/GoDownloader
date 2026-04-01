// Package utils provides shared utilities used across the application.
package utils

import (
	"net/http"
	"time"
)

const defaultUserAgent = "GoDownload/1.0"

// ClientOption is a functional option for configuring an HTTP client.
type ClientOption func(*http.Client)

// WithTimeout sets a custom request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *http.Client) {
		c.Timeout = d
	}
}

// NewHTTPClient returns an *http.Client with connection pooling, sensible
// transport settings, and an optional set of overrides.
//
// The returned client's Transport is a *http.Transport with:
//   - MaxIdleConns: 100
//   - MaxIdleConnsPerHost: 10
//   - IdleConnTimeout: 90 s
//   - TLSHandshakeTimeout: 10 s
//   - DisableCompression: false (accept gzip)
func NewHTTPClient(opts ...ClientOption) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  false,
	}

	c := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// NewRequest is a convenience wrapper around http.NewRequestWithContext that
// sets a default User-Agent header. callers may override the header after
// the call.
func NewRequest(method, url string, userAgent string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil) //nolint:noctx // callers add context via WithContext
	if err != nil {
		return nil, err
	}

	ua := userAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	req.Header.Set("User-Agent", ua)

	return req, nil
}
