package providers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const babepediaBaseURL = "https://www.babepedia.com/babe"

// Babepedia scrapes performer profile pages from babepedia.com.
type Babepedia struct {
	baseClient
}

// NewBabepedia creates a Babepedia scraper.
func NewBabepedia(client *http.Client, userAgent string) *Babepedia {
	return &Babepedia{baseClient: newBaseClient(client, userAgent)}
}

// SearchByName fetches the Babepedia profile page for the given performer name.
func (b *Babepedia) SearchByName(ctx context.Context, name string) (*PersonInfo, error) {
	// Babepedia uses underscores in URLs: "First_Last"
	slug := strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
	profileURL := fmt.Sprintf("%s/%s", babepediaBaseURL, slug)

	body, err := b.get(ctx, profileURL)
	if err != nil {
		return nil, fmt.Errorf("babepedia: fetching profile for %q: %w", name, err)
	}

	info := b.parse(body, name, slug)
	return &info, nil
}

// Regex patterns for Babepedia profile fields.
var (
	bpNameRe        = regexp.MustCompile(`(?i)<h1[^>]*id="babename"[^>]*>([^<]+)</h1>`)
	bpAliasRe       = regexp.MustCompile(`(?i)Aliases[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpBirthDateRe   = regexp.MustCompile(`(?i)Born[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpNationalityRe = regexp.MustCompile(`(?i)Birthplace[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpHairRe        = regexp.MustCompile(`(?i)Hair[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpEyeRe         = regexp.MustCompile(`(?i)Eyes[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpHeightRe      = regexp.MustCompile(`(?i)Height[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpWeightRe      = regexp.MustCompile(`(?i)Weight[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpMeasureRe     = regexp.MustCompile(`(?i)Measurements[:\s]*</td>\s*<td[^>]*>([^<]+)`)
	bpImageRe       = regexp.MustCompile(`(?i)<img[^>]+id="profimg"[^>]+src="([^"]+)"`)
)

func (b *Babepedia) parse(body, name, slug string) PersonInfo {
	info := PersonInfo{
		Name:       name,
		ExternalID: ptrStr(slug),
	}

	if m := bpNameRe.FindStringSubmatch(body); m != nil {
		info.Name = strings.TrimSpace(m[1])
	}

	if m := bpAliasRe.FindStringSubmatch(body); m != nil {
		aliases := strings.Split(m[1], ",")
		for _, a := range aliases {
			a = strings.TrimSpace(a)
			if a != "" {
				info.Aliases = append(info.Aliases, a)
			}
		}
	}

	if m := bpBirthDateRe.FindStringSubmatch(body); m != nil {
		info.BirthDate = parseDate(strings.TrimSpace(m[1]))
	}

	info.Nationality = extractField(bpNationalityRe, body)
	info.HairColor = extractField(bpHairRe, body)
	info.EyeColor = extractField(bpEyeRe, body)
	info.Height = extractField(bpHeightRe, body)
	info.Weight = extractField(bpWeightRe, body)
	info.Measurements = extractField(bpMeasureRe, body)

	if m := bpImageRe.FindStringSubmatch(body); m != nil {
		imgURL := m[1]
		// Babepedia may use relative paths.
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://www.babepedia.com" + imgURL
		}
		info.ImageURL = ptrStr(imgURL)
		if info.ImageURL != nil {
			info.ImageURLs = []string{*info.ImageURL}
		}
	}

	return info
}
