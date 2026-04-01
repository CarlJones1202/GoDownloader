package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ImxTo rips direct image URLs from imx.to image pages.
//
// imx.to uses a two-step interstitial flow:
//  1. GET the image page — may show a "Continue to your image..." button
//  2. If found, POST with imgContinue form data to get the actual image
//  3. Extract the full-size image from the response HTML
type ImxTo struct {
	userAgent string
}

// imxContinueRe detects the continue form input.
var imxContinueRe = regexp.MustCompile(`(?i)<input[^>]+name="imgContinue"[^>]+value="([^"]*)"`)

// imxImageRe matches images on the imx.to CDN.
var imxImageRe = regexp.MustCompile(`(?i)<img[^>]+src="(https?://[^"]*i\.imx\.to/[^"]*)"`)

// imxImageFallbackRe matches any imx.to image with /i/ path.
var imxImageFallbackRe = regexp.MustCompile(`(?i)<img[^>]+src="(https?://[^"]*imx\.to/[^"]*/i/[^"]*)"`)

// imxCentredRe is the original regex for the centred class img tag.
var imxCentredRe = regexp.MustCompile(`(?i)<img[^>]+class="centred(?:_resized)?"[^>]+src="(https?://[^"]+)"`)

// NewImxTo creates an ImxTo ripper.
func NewImxTo(_ *http.Client, userAgent string) *ImxTo {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"
	}
	return &ImxTo{userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *ImxTo) Hosts() []string {
	return []string{"imx.to", "www.imx.to"}
}

// Rip implements ripper.Ripper.
func (r *ImxTo) Rip(ctx context.Context, pageURL string) ([]string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}

	// Step 1: GET the image page.
	body, err := r.doGet(ctx, client, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imx.to: fetching page: %w", err)
	}

	// Try to find the image directly (some pages skip the interstitial).
	if imgURL := r.extractImage(body); imgURL != "" {
		return []string{imgURL}, nil
	}

	// Step 2: Check for continue button and POST if found.
	if m := imxContinueRe.FindStringSubmatch(body); m != nil {
		continueValue := m[1]
		body2, err := r.doPost(ctx, client, pageURL, map[string]string{
			"imgContinue": continueValue,
		})
		if err != nil {
			return nil, fmt.Errorf("imx.to: submitting continue form: %w", err)
		}

		if imgURL := r.extractImage(body2); imgURL != "" {
			return []string{imgURL}, nil
		}
	}

	// Step 3: HEAD request fallback — follow redirects.
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imx.to: building HEAD request: %w", err)
	}
	headReq.Header.Set("User-Agent", r.userAgent)

	resp, err := client.Do(headReq)
	if err != nil {
		return nil, fmt.Errorf("imx.to: HEAD fallback: %w", err)
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	imgURL := strings.ReplaceAll(finalURL, "/t/", "/i/")
	if imgURL != finalURL {
		return []string{imgURL}, nil
	}

	return nil, fmt.Errorf("imx.to: failed to extract image from %s", pageURL)
}

// extractImage tries multiple regexes to find the full-size image URL.
func (r *ImxTo) extractImage(body string) string {
	// Filter function to skip icons/logos/thumbs.
	isValidImg := func(src string) bool {
		lower := strings.ToLower(src)
		return !strings.Contains(lower, "icon") &&
			!strings.Contains(lower, "logo") &&
			!strings.Contains(lower, "avatar") &&
			!strings.Contains(lower, "thumb")
	}

	// Try centred class image first (most specific).
	if m := imxCentredRe.FindStringSubmatch(body); m != nil && isValidImg(m[1]) {
		return ensureAbsolute(m[1])
	}

	// Try CDN image (i.imx.to).
	if m := imxImageRe.FindStringSubmatch(body); m != nil && isValidImg(m[1]) {
		return ensureAbsolute(m[1])
	}

	// Try fallback /i/ path.
	if m := imxImageFallbackRe.FindStringSubmatch(body); m != nil && isValidImg(m[1]) {
		return ensureAbsolute(m[1])
	}

	// Scan all img tags for any imx.to CDN image.
	imgRe := regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"`)
	for _, m := range imgRe.FindAllStringSubmatch(body, -1) {
		src := m[1]
		if !isValidImg(src) {
			continue
		}
		if strings.Contains(src, "imx.to") && strings.Contains(src, "/i/") {
			return ensureAbsolute(src)
		}
	}

	return ""
}

// doGet performs an HTTP GET with proper headers.
func (r *ImxTo) doGet(ctx context.Context, client *http.Client, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// doPost performs an HTTP POST with form data.
func (r *ImxTo) doPost(ctx context.Context, client *http.Client, pageURL string, params map[string]string) (string, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pageURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", pageURL)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ensureAbsolute ensures a URL has an http(s) scheme.
func ensureAbsolute(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	if !strings.HasPrefix(u, "http") {
		return "https:" + u
	}
	return u
}
