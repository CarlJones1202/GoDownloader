package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const freeonesBaseURL = "https://www.freeones.com"

// FreeOnes scrapes performer profile pages from freeones.com.
type FreeOnes struct {
	baseClient
}

// NewFreeOnes creates a FreeOnes scraper.
func NewFreeOnes(client *http.Client, userAgent string) *FreeOnes {
	return &FreeOnes{baseClient: newBaseClient(client, userAgent)}
}

// SearchByName fetches the FreeOnes profile page for the given name and
// extracts whatever metadata is available.
func (f *FreeOnes) SearchByName(ctx context.Context, name string) (*PersonInfo, error) {
	slug := nameToSlug(name)
	profileURL := fmt.Sprintf("%s/%s/bio", freeonesBaseURL, slug)

	body, err := f.get(ctx, profileURL)
	if err != nil {
		return nil, fmt.Errorf("freeones: fetching profile for %q: %w", name, err)
	}

	info := f.parse(body, name, slug)
	return &info, nil
}

// GetBySlug fetches a performer by their FreeOnes slug directly.
func (f *FreeOnes) GetBySlug(ctx context.Context, slug string) (*PersonInfo, error) {
	profileURL := fmt.Sprintf("%s/%s/bio", freeonesBaseURL, slug)

	body, err := f.get(ctx, profileURL)
	if err != nil {
		return nil, fmt.Errorf("freeones: fetching %q: %w", slug, err)
	}

	info := f.parse(body, slug, slug)
	return &info, nil
}

// Regex patterns for extracting FreeOnes profile data.
var (
	foNameRe        = regexp.MustCompile(`(?i)<h1[^>]*>([^<]+)</h1>`)
	foAliasRe       = regexp.MustCompile(`(?i)Aliases[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foBirthDateRe   = regexp.MustCompile(`(?i)Date of Birth[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foNationalityRe = regexp.MustCompile(`(?i)Nationality[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foEthnicityRe   = regexp.MustCompile(`(?i)Ethnicity[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foHairRe        = regexp.MustCompile(`(?i)Hair Color[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foEyeRe         = regexp.MustCompile(`(?i)Eye Color[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foHeightRe      = regexp.MustCompile(`(?i)Height[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foWeightRe      = regexp.MustCompile(`(?i)Weight[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foMeasureRe     = regexp.MustCompile(`(?i)Measurements[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foTattoosRe     = regexp.MustCompile(`(?i)Tattoos[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foPiercingsRe   = regexp.MustCompile(`(?i)Piercings[^<]*</span>\s*<span[^>]*>([^<]+)`)
	foImageRe       = regexp.MustCompile(`(?i)<img[^>]+class="[^"]*profile[^"]*"[^>]+src="([^"]+)"`)
)

func (f *FreeOnes) parse(body, name, slug string) PersonInfo {
	info := PersonInfo{
		Name:       name,
		ExternalID: ptrStr(slug),
	}

	if m := foNameRe.FindStringSubmatch(body); m != nil {
		info.Name = strings.TrimSpace(m[1])
	}

	if m := foAliasRe.FindStringSubmatch(body); m != nil {
		aliases := strings.Split(m[1], ",")
		for _, a := range aliases {
			a = strings.TrimSpace(a)
			if a != "" {
				info.Aliases = append(info.Aliases, a)
			}
		}
	}

	if m := foBirthDateRe.FindStringSubmatch(body); m != nil {
		info.BirthDate = parseDate(strings.TrimSpace(m[1]))
	}

	info.Nationality = extractField(foNationalityRe, body)
	info.Ethnicity = extractField(foEthnicityRe, body)
	info.HairColor = extractField(foHairRe, body)
	info.EyeColor = extractField(foEyeRe, body)
	info.Height = extractField(foHeightRe, body)
	info.Weight = extractField(foWeightRe, body)
	info.Measurements = extractField(foMeasureRe, body)
	info.Tattoos = extractField(foTattoosRe, body)
	info.Piercings = extractField(foPiercingsRe, body)

	if m := foImageRe.FindStringSubmatch(body); m != nil {
		info.ImageURL = ptrStr(m[1])
	}

	return info
}

// extractField returns the first capture group of re in body, or nil.
func extractField(re *regexp.Regexp, body string) *string {
	m := re.FindStringSubmatch(body)
	if m == nil || len(m) < 2 {
		return nil
	}
	return ptrStr(m[1])
}

// nameToSlug converts "First Last" → "First-Last" for FreeOnes URLs.
func nameToSlug(name string) string {
	// URL-encode-safe slug: replace spaces with dashes, lowercase.
	slug := strings.ReplaceAll(strings.TrimSpace(name), " ", "-")
	return url.PathEscape(slug)
}
