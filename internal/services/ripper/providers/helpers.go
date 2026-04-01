package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/carlj/godownload/internal/utils"
)

// fetchPage performs a GET request and returns the response body as a string.
// The caller is responsible for providing a properly-contextualised request.
func fetchPage(ctx context.Context, client *http.Client, pageURL, userAgent string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("providers: building request for %q: %w", pageURL, err)
	}

	ua := userAgent
	if ua == "" {
		ua = "GoDownload/1.0"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("providers: GET %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("providers: GET %q returned %d", pageURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("providers: reading body from %q: %w", pageURL, err)
	}

	return string(body), nil
}

// firstMatch returns the first named capture group "url" from the first match
// of re in body. Returns an error if there is no match.
func firstMatch(re *regexp.Regexp, body, pageURL string) (string, error) {
	m := re.FindStringSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("providers: no match in page %q", pageURL)
	}

	// Prefer named group "url" if present.
	for i, name := range re.SubexpNames() {
		if name == "url" && i < len(m) && m[i] != "" {
			return m[i], nil
		}
	}

	// Fall back to the first capture group.
	if len(m) >= 2 && m[1] != "" {
		return m[1], nil
	}

	return "", fmt.Errorf("providers: empty capture in page %q", pageURL)
}

// newDefaultClient returns a shared HTTP client used by all provider rippers
// that don't receive an explicit client.
func newDefaultClient() *http.Client {
	return utils.NewHTTPClient()
}
