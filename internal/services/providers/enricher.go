// Package providers — Enricher aggregates metadata lookups across all providers.
package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Enricher coordinates metadata lookups across all available external providers
// and merges the results into a single PersonInfo.
type Enricher struct {
	stashDB   *StashDB
	freeones  *FreeOnes
	babepedia *Babepedia
	metart    *MetArt
	metartX   *MetArt
	playboy   *Playboy
}

// NewEnricher creates an Enricher with all provider clients initialised.
// vpnClient is optional — if non-nil it is used for age-gated providers
// (MetArt, MetArtX, Playboy); otherwise httpClient is used for everything.
func NewEnricher(httpClient *http.Client, userAgent, stashDBKey string, vpnClient *http.Client) *Enricher {
	gated := httpClient
	if vpnClient != nil {
		gated = vpnClient
	}

	return &Enricher{
		stashDB:   NewStashDB(httpClient, userAgent, stashDBKey),
		freeones:  NewFreeOnes(httpClient, userAgent),
		babepedia: NewBabepedia(httpClient, userAgent),
		metart:    NewMetArt(gated, userAgent),
		metartX:   NewMetArtX(gated, userAgent),
		playboy:   NewPlayboy(gated, userAgent),
	}
}

// ProviderResult holds the result from a single provider lookup.
type ProviderResult struct {
	Provider string      `json:"provider"`
	Person   *PersonInfo `json:"person,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// EnrichResult holds the merged person info and per-provider details.
type EnrichResult struct {
	Merged  PersonInfo       `json:"merged"`
	Sources []ProviderResult `json:"sources"`
}

// LookupPerson queries all providers for the given name and merges results.
// Individual provider failures are captured per-result, not propagated.
func (e *Enricher) LookupPerson(ctx context.Context, name string) *EnrichResult {
	type result struct {
		provider string
		info     *PersonInfo
		err      error
	}

	ch := make(chan result, 6)
	var wg sync.WaitGroup

	lookup := func(provider string, fn func(context.Context, string) (*PersonInfo, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Per-provider timeout so one slow provider doesn't block everything.
			pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			info, err := fn(pctx, name)
			ch <- result{provider: provider, info: info, err: err}
		}()
	}

	// StashDB returns a slice; wrap it to return the best match.
	lookup("stashdb", func(ctx context.Context, name string) (*PersonInfo, error) {
		results, err := e.stashDB.SearchByName(ctx, name)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no results")
		}
		return &results[0], nil
	})
	lookup("freeones", e.freeones.SearchByName)
	lookup("babepedia", e.babepedia.SearchByName)
	lookup("metart", func(ctx context.Context, name string) (*PersonInfo, error) {
		return e.metart.SearchModel(ctx, name)
	})
	lookup("metartx", func(ctx context.Context, name string) (*PersonInfo, error) {
		return e.metartX.SearchModel(ctx, name)
	})
	lookup("playboy", func(ctx context.Context, name string) (*PersonInfo, error) {
		return e.playboy.SearchModel(ctx, name)
	})

	go func() {
		wg.Wait()
		close(ch)
	}()

	var sources []ProviderResult
	var infos []*PersonInfo

	for r := range ch {
		pr := ProviderResult{Provider: r.provider}
		if r.err != nil {
			pr.Error = r.err.Error()
			slog.Debug("provider lookup failed", "provider", r.provider, "name", name, "error", r.err)
		} else {
			pr.Person = r.info
			infos = append(infos, r.info)
		}
		sources = append(sources, pr)
	}

	merged := mergePersonInfos(name, infos)

	return &EnrichResult{
		Merged:  merged,
		Sources: sources,
	}
}

// LookupProvider queries a single provider by name and returns the best match.
func (e *Enricher) LookupProvider(ctx context.Context, provider, name string) (*PersonInfo, error) {
	results, err := e.SearchProvider(ctx, provider, name)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%s: no results for %q", provider, name)
	}
	return &results[0], nil
}

// SearchProvider queries a single provider and returns ALL matching results.
// StashDB returns multiple candidates; scrapers (FreeOnes, Babepedia, etc.)
// return at most one result wrapped in a slice.
func (e *Enricher) SearchProvider(ctx context.Context, provider, name string) ([]PersonInfo, error) {
	switch strings.ToLower(provider) {
	case "stashdb":
		return e.stashDB.SearchByName(ctx, name)
	case "freeones":
		info, err := e.freeones.SearchByName(ctx, name)
		if err != nil {
			return nil, err
		}
		return []PersonInfo{*info}, nil
	case "babepedia":
		info, err := e.babepedia.SearchByName(ctx, name)
		if err != nil {
			return nil, err
		}
		return []PersonInfo{*info}, nil
	case "metart":
		info, err := e.metart.SearchModel(ctx, name)
		if err != nil {
			return nil, err
		}
		return []PersonInfo{*info}, nil
	case "metartx":
		info, err := e.metartX.SearchModel(ctx, name)
		if err != nil {
			return nil, err
		}
		return []PersonInfo{*info}, nil
	case "playboy":
		info, err := e.playboy.SearchModel(ctx, name)
		if err != nil {
			return nil, err
		}
		return []PersonInfo{*info}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
}

// GetByExternalID fetches a single performer from a provider by their external ID.
// Currently only StashDB supports this (via UUID); other providers return an error.
func (e *Enricher) GetByExternalID(ctx context.Context, provider, externalID string) (*PersonInfo, error) {
	switch strings.ToLower(provider) {
	case "stashdb":
		return e.stashDB.GetByID(ctx, externalID)
	case "freeones":
		return e.freeones.GetBySlug(ctx, externalID)
	default:
		return nil, fmt.Errorf("provider %q does not support lookup by external ID", provider)
	}
}

// ListProviders returns the names of all available providers.
func (e *Enricher) ListProviders() []string {
	return []string{"stashdb", "freeones", "babepedia", "metart", "metartx", "playboy"}
}

// mergePersonInfos picks the best non-nil value for each field across all results.
// Priority: first non-nil value wins (StashDB is typically first since it's the
// most comprehensive source).
func mergePersonInfos(name string, infos []*PersonInfo) PersonInfo {
	merged := PersonInfo{Name: name}

	for _, info := range infos {
		if info == nil {
			continue
		}

		// Prefer the provider's name over our search term if it differs.
		if merged.Name == name && info.Name != "" && info.Name != name {
			merged.Name = info.Name
		}

		if len(merged.Aliases) == 0 && len(info.Aliases) > 0 {
			merged.Aliases = info.Aliases
		} else if len(info.Aliases) > 0 {
			// Merge aliases, deduplicating.
			seen := make(map[string]struct{}, len(merged.Aliases))
			for _, a := range merged.Aliases {
				seen[strings.ToLower(a)] = struct{}{}
			}
			for _, a := range info.Aliases {
				if _, ok := seen[strings.ToLower(a)]; !ok {
					merged.Aliases = append(merged.Aliases, a)
					seen[strings.ToLower(a)] = struct{}{}
				}
			}
		}

		if merged.BirthDate == nil && info.BirthDate != nil {
			merged.BirthDate = info.BirthDate
		}
		if merged.Nationality == nil && info.Nationality != nil {
			merged.Nationality = info.Nationality
		}
		if merged.Ethnicity == nil && info.Ethnicity != nil {
			merged.Ethnicity = info.Ethnicity
		}
		if merged.HairColor == nil && info.HairColor != nil {
			merged.HairColor = info.HairColor
		}
		if merged.EyeColor == nil && info.EyeColor != nil {
			merged.EyeColor = info.EyeColor
		}
		if merged.Height == nil && info.Height != nil {
			merged.Height = info.Height
		}
		if merged.Weight == nil && info.Weight != nil {
			merged.Weight = info.Weight
		}
		if merged.Measurements == nil && info.Measurements != nil {
			merged.Measurements = info.Measurements
		}
		if merged.Tattoos == nil && info.Tattoos != nil {
			merged.Tattoos = info.Tattoos
		}
		if merged.Piercings == nil && info.Piercings != nil {
			merged.Piercings = info.Piercings
		}
		if merged.Biography == nil && info.Biography != nil {
			merged.Biography = info.Biography
		}
		if merged.ImageURL == nil && info.ImageURL != nil {
			merged.ImageURL = info.ImageURL
		}
	}

	return merged
}
