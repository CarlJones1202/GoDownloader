// Package providers implements clients for external metadata and gallery
// providers: StashDB (GraphQL), FreeOnes (scraper), Babepedia (scraper),
// MetArt (API), and Playboy (API).
//
// All providers share a common PersonInfo result type and a base HTTP client
// defined in this file.
package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/carlj/godownload/internal/utils"
)

// PersonInfo is the unified result returned by all metadata providers.
// Fields are pointers so providers only populate what they can extract.
type PersonInfo struct {
	Name         string
	Aliases      []string
	BirthDate    *time.Time
	Nationality  *string
	Ethnicity    *string
	HairColor    *string
	EyeColor     *string
	Height       *string // e.g. "170cm"
	Weight       *string // e.g. "55kg"
	Measurements *string // e.g. "34B-24-34"
	Tattoos      *string
	Piercings    *string
	Biography    *string
	ImageURL     *string // profile/headshot URL
	ExternalID   *string // provider-specific ID (StashDB UUID, slug, etc.)
}

// GalleryInfo is the unified result for gallery data from providers.
type GalleryInfo struct {
	Title        string
	URL          string
	Provider     string
	ProviderID   string
	ThumbnailURL *string
	Date         *time.Time
	Performers   []string // performer names found in the gallery
	ImageURLs    []string // direct image URLs (for providers that host images)
}

// baseClient wraps an *http.Client with a user-agent and provides convenience
// methods used by all provider implementations.
type baseClient struct {
	client    *http.Client
	userAgent string
}

// newBaseClient creates a baseClient. Pass nil for client to use defaults.
func newBaseClient(client *http.Client, userAgent string) baseClient {
	if client == nil {
		client = utils.NewHTTPClient(utils.WithTimeout(30 * time.Second))
	}
	if userAgent == "" {
		userAgent = "GoDownload/1.0"
	}
	return baseClient{client: client, userAgent: userAgent}
}

// get performs an HTTP GET and returns the response body as a string.
func (bc *baseClient) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("building request for %q: %w", url, err)
	}
	req.Header.Set("User-Agent", bc.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := bc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %q returned %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading body from %q: %w", url, err)
	}
	return string(body), nil
}

// postJSON sends a POST with the given JSON body and returns the response body.
func (bc *baseClient) postJSON(ctx context.Context, url, jsonBody string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("building POST request for %q: %w", url, err)
	}
	req.Header.Set("User-Agent", bc.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := bc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("POST %q returned %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading POST response from %q: %w", url, err)
	}
	return string(body), nil
}

// ptrStr returns a pointer to s, or nil if s is empty.
func ptrStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// parseDate attempts to parse a date string in common formats.
// Returns nil if parsing fails.
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	formats := []string{
		"2006-01-02",
		"January 2, 2006",
		"Jan 2, 2006",
		"02 January 2006",
		"2006/01/02",
		"01/02/2006",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}
