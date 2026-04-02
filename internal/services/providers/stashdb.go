package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

// graphQLRequest is the standard GraphQL request envelope.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// --- Response types matching StashDB schema (aligned with AG reference) ---

type stashDBPerformer struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Disambiguation string   `json:"disambiguation"`
	Aliases        []string `json:"aliases"`
	Gender         string   `json:"gender"`
	Birthdate      *struct {
		Date string `json:"date"`
	} `json:"birthdate"`
	Country      string `json:"country"`
	Height       *int   `json:"height"` // cm
	HairColor    string `json:"hair_color"`
	EyeColor     string `json:"eye_color"`
	Ethnicity    string `json:"ethnicity"`
	Measurements *struct {
		BandSize int    `json:"band_size"`
		CupSize  string `json:"cup_size"`
		Waist    int    `json:"waist"`
		Hip      int    `json:"hip"`
	} `json:"measurements"`
	Tattoos []struct {
		Location    string `json:"location"`
		Description string `json:"description"`
	} `json:"tattoos"`
	Piercings []struct {
		Location    string `json:"location"`
		Description string `json:"description"`
	} `json:"piercings"`
	URLs []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"urls"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

type stashDBSearchResponse struct {
	Data struct {
		QueryPerformers struct {
			Performers []stashDBPerformer `json:"performers"`
		} `json:"queryPerformers"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type stashDBGetResponse struct {
	Data struct {
		FindPerformer *stashDBPerformer `json:"findPerformer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// The performer fragment used in both search and get queries.
// Matches AG reference fields exactly.
const performerFragment = `
	id
	name
	disambiguation
	aliases
	gender
	birthdate { date }
	country
	height
	hair_color
	eye_color
	ethnicity
	measurements {
		band_size
		cup_size
		waist
		hip
	}
	tattoos {
		location
		description
	}
	piercings {
		location
		description
	}
	urls {
		url
		type
	}
	images { url }
`

// SearchByName searches StashDB for performers matching the given name.
// Uses queryPerformers (same as AG reference) for exact name matching.
func (s *StashDB) SearchByName(ctx context.Context, name string) ([]PersonInfo, error) {
	query := fmt.Sprintf(`
		query SearchPerformers($name: String!) {
			queryPerformers(input: {names: $name, page: 1, per_page: 20}) {
				performers { %s }
			}
		}
	`, performerFragment)

	var resp stashDBSearchResponse
	if err := s.execute(ctx, query, map[string]any{"name": name}, &resp); err != nil {
		return nil, fmt.Errorf("stashdb: search %q: %w", name, err)
	}

	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("stashdb: graphql error: %s", resp.Errors[0].Message)
	}

	results := make([]PersonInfo, 0, len(resp.Data.QueryPerformers.Performers))
	for _, p := range resp.Data.QueryPerformers.Performers {
		results = append(results, mapStashDBPerformer(p))
	}
	return results, nil
}

// GetByID fetches a single performer by their StashDB UUID.
func (s *StashDB) GetByID(ctx context.Context, id string) (*PersonInfo, error) {
	query := fmt.Sprintf(`
		query GetPerformer($id: ID!) {
			findPerformer(id: $id) { %s }
		}
	`, performerFragment)

	var resp stashDBGetResponse
	if err := s.execute(ctx, query, map[string]any{"id": id}, &resp); err != nil {
		return nil, fmt.Errorf("stashdb: get %q: %w", id, err)
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

// execute sends a GraphQL request to StashDB and decodes the response.
// This matches the AG reference implementation: proper JSON-marshalled
// {query, variables} body, ApiKey header for authentication.
func (s *StashDB) execute(ctx context.Context, query string, variables map[string]any, dest any) error {
	reqBody := graphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshalling graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stashDBEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.userAgent != "" {
		req.Header.Set("User-Agent", s.userAgent)
	}

	if s.apiKey != "" {
		req.Header.Set("ApiKey", s.apiKey)
	} else {
		slog.Warn("stashdb: no API key configured — request will likely fail with 'not authorized'")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %q: %w", stashDBEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stashdb returned status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decoding stashdb response: %w", err)
	}

	return nil
}

// mapStashDBPerformer converts a StashDB performer to our unified PersonInfo.
func mapStashDBPerformer(p stashDBPerformer) PersonInfo {
	info := PersonInfo{
		Name:       p.Name,
		Aliases:    p.Aliases,
		ExternalID: ptrStr(p.ID),
	}

	if p.Birthdate != nil && p.Birthdate.Date != "" {
		info.BirthDate = parseDate(p.Birthdate.Date)
	}
	info.Nationality = ptrStr(p.Country)
	info.Ethnicity = ptrStr(p.Ethnicity)
	info.HairColor = ptrStr(p.HairColor)
	info.EyeColor = ptrStr(p.EyeColor)

	if p.Height != nil && *p.Height > 0 {
		h := fmt.Sprintf("%dcm", *p.Height)
		info.Height = &h
	}

	// Format measurements like AG: "Band: 32, Cup: B, Waist: 24, Hip: 34"
	if p.Measurements != nil {
		m := fmt.Sprintf("%d%s-%d-%d",
			p.Measurements.BandSize, p.Measurements.CupSize,
			p.Measurements.Waist, p.Measurements.Hip)
		// Only set if we have meaningful data (not all zeros).
		if p.Measurements.BandSize > 0 || p.Measurements.CupSize != "" {
			info.Measurements = &m
		}
	}

	// Concatenate tattoos with location.
	if len(p.Tattoos) > 0 {
		descs := make([]string, 0, len(p.Tattoos))
		for _, t := range p.Tattoos {
			if t.Description != "" && t.Location != "" {
				descs = append(descs, fmt.Sprintf("%s (%s)", t.Description, t.Location))
			} else if t.Description != "" {
				descs = append(descs, t.Description)
			} else if t.Location != "" {
				descs = append(descs, t.Location)
			}
		}
		if len(descs) > 0 {
			joined := joinNonEmpty(descs, "; ")
			info.Tattoos = ptrStr(joined)
		}
	}

	// Concatenate piercings with location.
	if len(p.Piercings) > 0 {
		descs := make([]string, 0, len(p.Piercings))
		for _, pi := range p.Piercings {
			if pi.Description != "" && pi.Location != "" {
				descs = append(descs, fmt.Sprintf("%s (%s)", pi.Description, pi.Location))
			} else if pi.Description != "" {
				descs = append(descs, pi.Description)
			} else if pi.Location != "" {
				descs = append(descs, pi.Location)
			}
		}
		if len(descs) > 0 {
			joined := joinNonEmpty(descs, "; ")
			info.Piercings = ptrStr(joined)
		}
	}

	// Collect ALL image URLs for photo downloads.
	if len(p.Images) > 0 {
		for _, img := range p.Images {
			if img.URL != "" {
				info.ImageURLs = append(info.ImageURLs, img.URL)
			}
		}
		// Primary image is the first one.
		if len(info.ImageURLs) > 0 {
			info.ImageURL = &info.ImageURLs[0]
		}
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
