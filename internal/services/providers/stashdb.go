package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	stashDBEndpoint = "https://stashdb.org/graphql"
)

// StashDB queries the StashDB GraphQL API for performer metadata.
type StashDB struct {
	baseClient
	apiKey string // optional API key for authenticated queries
}

// NewStashDB creates a StashDB client. Pass an empty apiKey for
// unauthenticated (rate-limited) access.
func NewStashDB(client *http.Client, userAgent, apiKey string) *StashDB {
	return &StashDB{
		baseClient: newBaseClient(client, userAgent),
		apiKey:     apiKey,
	}
}

// stashDBPerformerResponse is the top-level GraphQL response shape.
type stashDBPerformerResponse struct {
	Data struct {
		FindPerformer *stashDBPerformer `json:"findPerformer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type stashDBSearchResponse struct {
	Data struct {
		SearchPerformer []stashDBPerformer `json:"searchPerformer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type stashDBPerformer struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	BirthDate    *string  `json:"birth_date"`
	Country      *string  `json:"country"`
	Ethnicity    *string  `json:"ethnicity"`
	HairColor    *string  `json:"hair_color"`
	EyeColor     *string  `json:"eye_color"`
	Height       *int     `json:"height"` // cm
	Weight       *int     `json:"weight"` // kg
	Measurements *string  `json:"measurements"`
	Tattoos      []struct {
		Description string `json:"description"`
	} `json:"tattoos"`
	Piercings []struct {
		Description string `json:"description"`
	} `json:"piercings"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

const performerFragment = `
	id
	name
	aliases
	birth_date
	country
	ethnicity
	hair_color
	eye_color
	height
	weight
	measurements
	tattoos { description }
	piercings { description }
	images { url }
`

// SearchByName searches StashDB for performers matching the given name.
func (s *StashDB) SearchByName(ctx context.Context, name string) ([]PersonInfo, error) {
	query := fmt.Sprintf(`{
		"query": "query { searchPerformer(term: %q) { %s } }"
	}`, name, performerFragment)

	body, err := s.graphql(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("stashdb: search %q: %w", name, err)
	}

	var resp stashDBSearchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("stashdb: decoding search response: %w", err)
	}

	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("stashdb: graphql error: %s", resp.Errors[0].Message)
	}

	results := make([]PersonInfo, 0, len(resp.Data.SearchPerformer))
	for _, p := range resp.Data.SearchPerformer {
		results = append(results, mapStashDBPerformer(p))
	}
	return results, nil
}

// GetByID fetches a single performer by their StashDB UUID.
func (s *StashDB) GetByID(ctx context.Context, id string) (*PersonInfo, error) {
	query := fmt.Sprintf(`{
		"query": "query { findPerformer(id: %q) { %s } }"
	}`, id, performerFragment)

	body, err := s.graphql(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("stashdb: get %q: %w", id, err)
	}

	var resp stashDBPerformerResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("stashdb: decoding response: %w", err)
	}

	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("stashdb: graphql error: %s", resp.Errors[0].Message)
	}

	if resp.Data.FindPerformer == nil {
		return nil, fmt.Errorf("stashdb: performer %q not found", id)
	}

	info := mapStashDBPerformer(*resp.Data.FindPerformer)
	return &info, nil
}

// graphql sends a GraphQL request to the StashDB endpoint.
func (s *StashDB) graphql(ctx context.Context, jsonBody string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stashDBEndpoint, nil)
	if err != nil {
		return "", err
	}

	// If we have an API key, attach it.
	if s.apiKey != "" {
		req.Header.Set("ApiKey", s.apiKey)
	}

	return s.postJSON(ctx, stashDBEndpoint, jsonBody)
}

// mapStashDBPerformer converts a StashDB performer to our unified PersonInfo.
func mapStashDBPerformer(p stashDBPerformer) PersonInfo {
	info := PersonInfo{
		Name:       p.Name,
		Aliases:    p.Aliases,
		ExternalID: ptrStr(p.ID),
	}

	if p.BirthDate != nil {
		info.BirthDate = parseDate(*p.BirthDate)
	}
	info.Nationality = p.Country
	info.Ethnicity = p.Ethnicity
	info.HairColor = p.HairColor
	info.EyeColor = p.EyeColor

	if p.Height != nil {
		h := fmt.Sprintf("%dcm", *p.Height)
		info.Height = &h
	}
	if p.Weight != nil {
		w := fmt.Sprintf("%dkg", *p.Weight)
		info.Weight = &w
	}
	info.Measurements = p.Measurements

	// Concatenate tattoos.
	if len(p.Tattoos) > 0 {
		descs := make([]string, len(p.Tattoos))
		for i, t := range p.Tattoos {
			descs[i] = t.Description
		}
		joined := joinNonEmpty(descs, "; ")
		info.Tattoos = ptrStr(joined)
	}

	// Concatenate piercings.
	if len(p.Piercings) > 0 {
		descs := make([]string, len(p.Piercings))
		for i, pi := range p.Piercings {
			descs[i] = pi.Description
		}
		joined := joinNonEmpty(descs, "; ")
		info.Piercings = ptrStr(joined)
	}

	// Take the first image.
	if len(p.Images) > 0 && p.Images[0].URL != "" {
		info.ImageURL = &p.Images[0].URL
	}

	return info
}

// joinNonEmpty joins non-empty strings with sep.
func joinNonEmpty(ss []string, sep string) string {
	var filtered []string
	for _, s := range ss {
		if s != "" {
			filtered = append(filtered, s)
		}
	}
	result := ""
	for i, s := range filtered {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
