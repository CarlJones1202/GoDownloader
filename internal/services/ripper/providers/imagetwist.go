package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Imagetwist rips direct image URLs from imagetwist.com image pages.
//
// Imagetwist uses the same interstitial pattern as imx.to:
//  1. GET the image page — may show a "Continue" button
//  2. POST with imgContinue to get the actual image
//  3. Extract img.pic or img.img-responsive from the response
type Imagetwist struct {
	userAgent string
}

// itwContinueRe detects the imgContinue form input.
var itwContinueRe = regexp.MustCompile(`(?i)<input[^>]+name="imgContinue"[^>]+value="([^"]*)"`)

// itwContinueBtnRe detects a generic continue button.
var itwContinueBtnRe = regexp.MustCompile(`(?i)<input[^>]+class="[^"]*btn-success[^"]*"[^>]+value="([^"]*(?:continue|image)[^"]*)"`)

// itwPicRe matches img.pic elements.
var itwPicRe = regexp.MustCompile(`(?i)<img[^>]+class="pic"[^>]+src="([^"]+)"`)

// itwResponsiveRe matches img.img-responsive elements (fallback).
var itwResponsiveRe = regexp.MustCompile(`(?i)<img[^>]+class="img-responsive"[^>]+src="([^"]+)"`)

// NewImagetwist creates an Imagetwist ripper.
func NewImagetwist(_ *http.Client, userAgent string) *Imagetwist {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"
	}
	return &Imagetwist{userAgent: userAgent}
}

// Hosts implements ripper.Ripper.
func (r *Imagetwist) Hosts() []string {
	return []string{"imagetwist.com", "www.imagetwist.com"}
}

// Rip implements ripper.Ripper.
func (r *Imagetwist) Rip(ctx context.Context, pageURL string) ([]string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
	}

	// Step 1: GET the image page. This first request often just sets a session cookie
	// and doesn't return the actual image HTML (as seen in gallery-dl's behavior).
	_, err := r.doGet(ctx, client, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imagetwist: fetching cookie page: %w", err)
	}

	// Step 2: GET the page AGAIN now that the cookie jar is populated.
	body, err := r.doGet(ctx, client, pageURL)
	if err != nil {
		return nil, fmt.Errorf("imagetwist: fetching image page: %w", err)
	}

	// Try to find the image directly.
	if imgURL := r.extractImage(body); imgURL != "" {
		return []string{imgURL}, nil
	}

	// Step 2: Check for continue button and POST.
	params := map[string]string{}

	if m := itwContinueRe.FindStringSubmatch(body); m != nil {
		params["imgContinue"] = m[1]
	} else if m := itwContinueBtnRe.FindStringSubmatch(body); m != nil {
		// Generic continue button — post with default imgContinue.
		params["imgContinue"] = "Continue to image ..."
	} else {
		_ = os.WriteFile("imagetwist_dump.html", []byte(body), 0644)
		return nil, fmt.Errorf("imagetwist: no image or continue button found on %s (dumped to imagetwist_dump.html)", pageURL)
	}

	body2, err := r.doPost(ctx, client, pageURL, params)
	if err != nil {
		return nil, fmt.Errorf("imagetwist: submitting continue form: %w", err)
	}

	if imgURL := r.extractImage(body2); imgURL != "" {
		return []string{imgURL}, nil
	}

	return nil, fmt.Errorf("imagetwist: failed to extract image from %s", pageURL)
}

// extractImage tries multiple selectors to find the image URL.
func (r *Imagetwist) extractImage(body string) string {
	isValid := func(src string) bool {
		lower := strings.ToLower(src)
		return !strings.Contains(lower, "logo") &&
			!strings.Contains(lower, "icon") &&
			!strings.Contains(lower, "avatar")
	}

	if m := itwPicRe.FindStringSubmatch(body); m != nil && isValid(m[1]) {
		return itwEnsureAbsolute(m[1])
	}

	if m := itwResponsiveRe.FindStringSubmatch(body); m != nil && isValid(m[1]) {
		return itwEnsureAbsolute(m[1])
	}

	// Fallback (gallery-dl behavior): Just grab the first valid <img src="..."> on the page.
	// ImageTwist frequently changes or drops their CSS classes.
	fallbackRe := regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"`)
	for _, m := range fallbackRe.FindAllStringSubmatch(body, -1) {
		if isValid(m[1]) && !strings.Contains(m[1], "/imgs/") {
			return itwEnsureAbsolute(m[1])
		}
	}

	return ""
}

func (r *Imagetwist) doGet(ctx context.Context, client *http.Client, pageURL string) (string, error) {
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

func (r *Imagetwist) doPost(ctx context.Context, client *http.Client, pageURL string, params map[string]string) (string, error) {
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

func itwEnsureAbsolute(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	if !strings.HasPrefix(u, "http") {
		return "https:" + u
	}
	return u
}
