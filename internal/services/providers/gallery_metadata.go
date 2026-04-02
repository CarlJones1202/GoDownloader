// Package providers — GalleryMetadataService searches and scrapes gallery
// metadata from 12 external providers. It uses VPN-aware HTTP clients for
// age-gated domains (MetArt network, Playboy, MPLStudios, etc.).
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// GallerySearchResult represents a search result candidate from a provider.
type GallerySearchResult struct {
	Provider    string `json:"provider"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Thumbnail   string `json:"thumbnail"`
	ReleaseDate string `json:"release_date,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
}

// GalleryMetadata represents scraped metadata from a confirmed gallery.
type GalleryMetadata struct {
	Provider     string    `json:"provider"`
	Description  string    `json:"description"`
	Rating       float64   `json:"rating"`
	ReleaseDate  time.Time `json:"release_date"`
	SourceURL    string    `json:"source_url"`
	ThumbnailURL string    `json:"thumbnail_url"`
}

// HTTPClientFunc returns an HTTP client appropriate for a target URL.
// This allows the service to use VPN-aware clients for age-gated domains.
type HTTPClientFunc func(targetURL string) *http.Client

// maxResultsPerProvider limits how many results any single provider can
// contribute to a search, preventing aggressive scrapers from flooding.
const maxResultsPerProvider = 15

// GalleryMetadataService coordinates gallery metadata search and scrape
// operations across all supported providers.
type GalleryMetadataService struct {
	getClient HTTPClientFunc
	userAgent string
}

// NewGalleryMetadataService creates a new service. getClient should return
// a VPN-routed client for age-gated URLs and a direct client otherwise.
func NewGalleryMetadataService(getClient HTTPClientFunc, userAgent string) *GalleryMetadataService {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	return &GalleryMetadataService{
		getClient: getClient,
		userAgent: userAgent,
	}
}

// SearchAll searches all providers concurrently for galleries matching query.
func (s *GalleryMetadataService) SearchAll(ctx context.Context, query string) ([]GallerySearchResult, error) {
	return s.searchProviders(ctx, query, "")
}

// SearchByProvider searches a single provider for galleries matching query.
// The provider name is case-insensitive.
func (s *GalleryMetadataService) SearchByProvider(ctx context.Context, query, provider string) ([]GallerySearchResult, error) {
	return s.searchProviders(ctx, query, provider)
}

// ProviderNames returns the list of supported provider names.
func (s *GalleryMetadataService) ProviderNames() []string {
	return []string{
		"MetArt", "MetartX", "SexArt", "LifeErotic", "EternalDesire", "RylskyArt",
		"Playboy", "PlayboyPlus", "Vixen", "VivThomas", "WowGirls", "MPLStudios",
	}
}

func (s *GalleryMetadataService) searchProviders(ctx context.Context, query, filterProvider string) ([]GallerySearchResult, error) {
	type providerResult struct {
		results []GallerySearchResult
		err     error
		name    string
	}

	allProviders := []struct {
		name string
		fn   func(context.Context, string) ([]GallerySearchResult, error)
	}{
		{"MetArt", s.searchMetArtNetwork(ctx, "MetArt", "https://www.metart.com")},
		{"MetartX", s.searchMetArtNetwork(ctx, "MetartX", "https://www.metartx.com")},
		{"SexArt", s.searchMetArtNetwork(ctx, "SexArt", "https://www.sexart.com")},
		{"LifeErotic", s.searchMetArtNetwork(ctx, "LifeErotic", "https://www.thelifeerotic.com")},
		{"EternalDesire", s.searchMetArtNetwork(ctx, "EternalDesire", "https://www.eternaldesire.com")},
		{"RylskyArt", s.searchMetArtNetwork(ctx, "RylskyArt", "https://www.rylskyart.com")},
		{"Playboy", s.searchPlayboy},
		{"PlayboyPlus", s.searchPlayboyPlus},
		{"Vixen", s.searchVixen},
		{"VivThomas", s.searchVivThomas},
		{"WowGirls", s.searchWowGirls},
		{"MPLStudios", s.searchMPLStudios},
	}

	// Filter to a single provider if specified.
	var providers []struct {
		name string
		fn   func(context.Context, string) ([]GallerySearchResult, error)
	}
	if filterProvider != "" {
		for _, p := range allProviders {
			if strings.EqualFold(p.name, filterProvider) {
				providers = append(providers, p)
				break
			}
		}
		if len(providers) == 0 {
			return nil, fmt.Errorf("unsupported provider: %s", filterProvider)
		}
	} else {
		providers = allProviders
	}

	ch := make(chan providerResult, len(providers))
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(name string, fn func(context.Context, string) ([]GallerySearchResult, error)) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			results, err := fn(pctx, query)
			ch <- providerResult{results: results, err: err, name: name}
		}(p.name, p.fn)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []GallerySearchResult
	for pr := range ch {
		if pr.err != nil {
			slog.Warn("gallery metadata search failed", "provider", pr.name, "error", pr.err)
			continue
		}
		// Cap results per provider to prevent any single provider from flooding.
		results := pr.results
		if len(results) > maxResultsPerProvider {
			results = results[:maxResultsPerProvider]
		}
		all = append(all, results...)
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no matching galleries found for %q", query)
	}

	slog.Info("gallery metadata search complete", "query", query, "results", len(all))
	return all, nil
}

// ScrapeMetadata scrapes full metadata from a confirmed gallery URL.
func (s *GalleryMetadataService) ScrapeMetadata(ctx context.Context, sourceURL, provider, sourceID string) (*GalleryMetadata, error) {
	slog.Info("scraping gallery metadata", "provider", provider, "url", sourceURL)

	switch strings.ToLower(provider) {
	case "metart":
		return s.scrapeMetArtNetwork(ctx, "MetArt", "https://www.metart.com", sourceURL)
	case "metartx":
		return s.scrapeMetArtNetwork(ctx, "MetartX", "https://www.metartx.com", sourceURL)
	case "sexart":
		return s.scrapeMetArtNetwork(ctx, "SexArt", "https://www.sexart.com", sourceURL)
	case "lifeerotic":
		return s.scrapeMetArtNetwork(ctx, "LifeErotic", "https://www.thelifeerotic.com", sourceURL)
	case "eternaldesire":
		return s.scrapeMetArtNetwork(ctx, "EternalDesire", "https://www.eternaldesire.com", sourceURL)
	case "rylskyart":
		return s.scrapeRylskyArt(ctx, sourceURL)
	case "playboy":
		return s.scrapePlayboy(ctx, sourceURL)
	case "playboyplus":
		return s.scrapePlayboyPlus(ctx, sourceURL)
	case "vixen":
		return s.scrapeVixen(ctx, sourceURL)
	case "vivthomas":
		return s.scrapeHTMLGallery(ctx, "VivThomas", sourceURL)
	case "wowgirls":
		return s.scrapeHTMLGallery(ctx, "WowGirls", sourceURL)
	case "mplstudios":
		return s.scrapeHTMLGallery(ctx, "MPLStudios", sourceURL)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// ---------------------------------------------------------------------------
// MetArt network (6 sites, identical API)
// ---------------------------------------------------------------------------

// searchMetArtNetwork returns a closure that searches a MetArt-network site.
// All 6 sites (MetArt, MetartX, SexArt, LifeErotic, EternalDesire, RylskyArt)
// use the same /api/search-results JSON API.
func (s *GalleryMetadataService) searchMetArtNetwork(_ context.Context, provider, baseURL string) func(context.Context, string) ([]GallerySearchResult, error) {
	return func(ctx context.Context, query string) ([]GallerySearchResult, error) {
		apiURL := fmt.Sprintf("%s/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery",
			baseURL, url.QueryEscape(query))

		body, err := s.httpGet(ctx, apiURL)
		if err != nil {
			return nil, fmt.Errorf("%s search: %w", provider, err)
		}

		// The MetArt API returns an array of category objects, e.g.:
		// [{"displayName":"Galleries","total":5,"items":[{"score":68,"type":"GALLERY","item":{...}}]}, ...]
		// In some cases it may return a single category object (not wrapped in an array).
		type searchItem struct {
			Score float64 `json:"score"`
			Type  string  `json:"type"`
			Item  struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				PublishedAt string `json:"publishedAt"`
				Thumbnail   string `json:"thumbnailCoverPath"`
			} `json:"item"`
		}
		type searchCategory struct {
			DisplayName string       `json:"displayName"`
			Items       []searchItem `json:"items"`
		}

		var categories []searchCategory

		// Try parsing as array first, then as single object.
		if err := json.Unmarshal(body, &categories); err != nil {
			var single searchCategory
			if err2 := json.Unmarshal(body, &single); err2 != nil {
				return nil, fmt.Errorf("%s: parsing search JSON (array: %v, object: %v)", provider, err, err2)
			}
			categories = []searchCategory{single}
		}

		slog.Info("metart network parse", "provider", provider, "categories", len(categories),
			"raw_len", len(body))

		var results []GallerySearchResult
		for _, cat := range categories {
			slog.Info("metart category", "provider", provider, "displayName", cat.DisplayName,
				"items", len(cat.Items))
			for _, entry := range cat.Items {
				// Only include GALLERY type items (skip MOVIE, MODEL, etc.)
				if entry.Type != "" && entry.Type != "GALLERY" {
					continue
				}

				item := entry.Item

				// RylskyArt returns non-gallery items too — filter by path.
				if provider == "RylskyArt" && !strings.Contains(strings.ToLower(item.Path), "/gallery/") {
					continue
				}

				galleryURL := baseURL + item.Path
				thumbURL := baseURL + item.Thumbnail

				dateStr := item.PublishedAt
				if len(dateStr) > 10 {
					dateStr = dateStr[:10]
				}

				results = append(results, GallerySearchResult{
					Provider:    provider,
					Title:       item.Name,
					URL:         galleryURL,
					Thumbnail:   thumbURL,
					ReleaseDate: dateStr,
				})
			}
		}

		slog.Info("metart network search", "provider", provider, "results", len(results))
		return results, nil
	}
}

// scrapeMetArtNetwork scrapes metadata from a MetArt-network gallery.
// URL format: .../model/{name}/gallery/{date}/{galleryName}
// API: /api/gallery?name={galleryName}&date={date}
func (s *GalleryMetadataService) scrapeMetArtNetwork(ctx context.Context, provider, baseURL, sourceURL string) (*GalleryMetadata, error) {
	galleryDate, galleryName, err := parseMetArtGalleryURL(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", provider, err)
	}

	apiURL := fmt.Sprintf("%s/api/gallery?name=%s&date=%s", baseURL, galleryName, galleryDate)

	body, err := s.httpGet(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("%s scrape: %w", provider, err)
	}

	var detail struct {
		Name                string  `json:"name"`
		Description         string  `json:"description"`
		MetaDescription     string  `json:"metaDescription"`
		RatingAverage       float64 `json:"ratingAverage"`
		PublishedAt         string  `json:"publishedAt"`
		CoverCleanImagePath string  `json:"coverCleanImagePath"`
		CoverImagePath      string  `json:"coverImagePath"`
		CoverImageURL       string  `json:"coverImageUrl"`
		CoverImage          string  `json:"coverImage"`
		Cover               string  `json:"cover"`
	}

	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("%s: parsing gallery detail: %w", provider, err)
	}

	metadata := &GalleryMetadata{
		Provider:    provider,
		SourceURL:   sourceURL,
		Description: detail.Description,
		Rating:      detail.RatingAverage,
	}

	// RylskyArt uses metaDescription instead of description.
	if metadata.Description == "" && detail.MetaDescription != "" {
		metadata.Description = detail.MetaDescription
	}

	// Find best thumbnail from available cover fields.
	for _, candidate := range []string{
		detail.CoverCleanImagePath,
		detail.CoverImagePath,
		detail.CoverImageURL,
		detail.CoverImage,
		detail.Cover,
	} {
		if candidate != "" {
			metadata.ThumbnailURL = toAbsoluteURL(baseURL, candidate)
			break
		}
	}

	if metadata.ThumbnailURL == "" {
		// Fallback: construct from gallery name.
		metadata.ThumbnailURL = fmt.Sprintf("%s/photo/%s/0/0.jpg", baseURL, galleryName)
	}

	if detail.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", detail.PublishedAt); err == nil {
			metadata.ReleaseDate = parsed
		} else if len(detail.PublishedAt) >= 10 {
			if parsed, err := time.Parse("2006-01-02", detail.PublishedAt[:10]); err == nil {
				metadata.ReleaseDate = parsed
			}
		}
	}

	return metadata, nil
}

// scrapeRylskyArt is a specialised scraper for RylskyArt which uses a slightly
// different JSON field name (metaDescription instead of description).
func (s *GalleryMetadataService) scrapeRylskyArt(ctx context.Context, sourceURL string) (*GalleryMetadata, error) {
	return s.scrapeMetArtNetwork(ctx, "RylskyArt", "https://www.rylskyart.com", sourceURL)
}

// ---------------------------------------------------------------------------
// Playboy (HTML scraping)
// ---------------------------------------------------------------------------

func (s *GalleryMetadataService) searchPlayboy(ctx context.Context, query string) ([]GallerySearchResult, error) {
	searchURL := fmt.Sprintf("https://www.playboy.com/search?q=%s", strings.ReplaceAll(query, " ", "+"))

	body, err := s.httpGet(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("Playboy search: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("Playboy: parsing HTML: %w", err)
	}

	var results []GallerySearchResult
	doc.Find(".gallery-item, .search-result, article").Each(func(i int, sel *goquery.Selection) {
		if i >= 10 {
			return
		}
		title := strings.TrimSpace(sel.Find("h2, h3, .title").First().Text())
		href, _ := sel.Find("a").First().Attr("href")
		thumbnail, _ := sel.Find("img").First().Attr("src")
		releaseDate := strings.TrimSpace(sel.Find(".date, time").First().Text())

		if href != "" && !strings.HasPrefix(href, "http") {
			href = "https://www.playboy.com" + href
		}
		if thumbnail != "" && !strings.HasPrefix(thumbnail, "http") {
			thumbnail = "https://www.playboy.com" + thumbnail
		}
		if title != "" && href != "" {
			results = append(results, GallerySearchResult{
				Provider:    "Playboy",
				Title:       title,
				URL:         href,
				Thumbnail:   thumbnail,
				ReleaseDate: releaseDate,
			})
		}
	})

	return results, nil
}

func (s *GalleryMetadataService) scrapePlayboy(ctx context.Context, sourceURL string) (*GalleryMetadata, error) {
	body, err := s.httpGet(ctx, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("Playboy scrape: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("Playboy: parsing HTML: %w", err)
	}

	metadata := &GalleryMetadata{Provider: "Playboy", SourceURL: sourceURL}
	metadata.Description = strings.TrimSpace(doc.Find(".description, .synopsis, p.description").First().Text())

	ratingText := strings.TrimSpace(doc.Find(".rating, .score").First().Text())
	if ratingText != "" {
		fmt.Sscanf(ratingText, "%f", &metadata.Rating)
	}

	metadata.ReleaseDate = parseDateFromDoc(doc)

	return metadata, nil
}

// ---------------------------------------------------------------------------
// PlayboyPlus (Algolia API)
// ---------------------------------------------------------------------------

const (
	algoliaAppID  = "TSMKFA364Q"
	algoliaAPIKey = "MDJmMzNkZTQ5YzY1NGFkOGY5NDU1OTU5M2Y4ZGFhNDdiZDM4N2QwZjY1ZWNmODkyOWRlNzE0NjRlNTVmYzNhNnZhbGlkVW50aWw9MTc3MjIzNjk3OCZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"
)

func (s *GalleryMetadataService) searchPlayboyPlus(ctx context.Context, query string) ([]GallerySearchResult, error) {
	apiURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/all_photosets/query", algoliaAppID)

	reqBody := fmt.Sprintf(`{"params":"query=%s&hitsPerPage=20&filters=segment:playboyplus"}`,
		url.QueryEscape(query))

	body, err := s.algoliaPost(ctx, apiURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("PlayboyPlus search: %w", err)
	}

	var apiResp struct {
		Hits []struct {
			ObjectID    string `json:"objectID"`
			Title       string `json:"title"`
			URLTitle    string `json:"urlTitle"`
			ReleaseDate string `json:"release_date"`
			Thumbnails  struct {
				Standard string `json:"standard"`
			} `json:"thumbnails"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("PlayboyPlus: parsing response: %w", err)
	}

	var results []GallerySearchResult
	for _, hit := range apiResp.Hits {
		results = append(results, GallerySearchResult{
			Provider:    "PlayboyPlus",
			Title:       hit.Title,
			URL:         fmt.Sprintf("https://www.playboyplus.com/en/update/%s/%s", hit.URLTitle, hit.ObjectID),
			Thumbnail:   hit.Thumbnails.Standard,
			ReleaseDate: hit.ReleaseDate,
			SourceID:    hit.ObjectID,
		})
	}

	return results, nil
}

func (s *GalleryMetadataService) scrapePlayboyPlus(ctx context.Context, sourceURL string) (*GalleryMetadata, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PlayboyPlus URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid PlayboyPlus URL format")
	}
	objectID := parts[len(parts)-1]

	apiURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/all_photosets/%s", algoliaAppID, objectID)

	body, err := s.algoliaGet(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("PlayboyPlus scrape: %w", err)
	}

	var hit struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		ReleaseDate string `json:"release_date"`
	}

	if err := json.Unmarshal(body, &hit); err != nil {
		return nil, fmt.Errorf("PlayboyPlus: parsing record: %w", err)
	}

	metadata := &GalleryMetadata{
		Provider:    "PlayboyPlus",
		SourceURL:   sourceURL,
		Description: hit.Description,
	}

	if hit.ReleaseDate != "" && len(hit.ReleaseDate) >= 10 {
		if parsed, err := time.Parse("2006-01-02", hit.ReleaseDate[:10]); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	return metadata, nil
}

// ---------------------------------------------------------------------------
// Vixen (GraphQL API)
// ---------------------------------------------------------------------------

func (s *GalleryMetadataService) searchVixen(ctx context.Context, query string) ([]GallerySearchResult, error) {
	gqlQuery := map[string]any{
		"query": `query search($term: String) {
			search(term: $term) {
				videos {
					id
					title
					slug
					releaseDate
					images {
						poster {
							url
						}
					}
				}
			}
		}`,
		"variables": map[string]string{"term": query},
	}

	apiURL := "https://www.vixen.com/graphql"
	body, err := s.httpPostJSON(ctx, apiURL, gqlQuery)
	if err != nil {
		return nil, fmt.Errorf("Vixen search: %w", err)
	}

	var apiResp struct {
		Data struct {
			Search struct {
				Videos []struct {
					ID          string `json:"id"`
					Title       string `json:"title"`
					Slug        string `json:"slug"`
					ReleaseDate string `json:"releaseDate"`
					Images      struct {
						Poster []struct {
							URL string `json:"url"`
						} `json:"poster"`
					} `json:"images"`
				} `json:"videos"`
			} `json:"search"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("Vixen: parsing response: %w", err)
	}

	var results []GallerySearchResult
	for _, video := range apiResp.Data.Search.Videos {
		thumbURL := ""
		if len(video.Images.Poster) > 0 {
			thumbURL = video.Images.Poster[0].URL
		}
		results = append(results, GallerySearchResult{
			Provider:    "Vixen",
			Title:       video.Title,
			URL:         "https://www.vixen.com/videos/" + video.Slug,
			Thumbnail:   thumbURL,
			ReleaseDate: video.ReleaseDate,
			SourceID:    video.ID,
		})
	}

	return results, nil
}

func (s *GalleryMetadataService) scrapeVixen(ctx context.Context, sourceURL string) (*GalleryMetadata, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Vixen URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "videos" {
		return nil, fmt.Errorf("invalid Vixen video URL format")
	}
	slug := parts[len(parts)-1]

	gqlQuery := map[string]any{
		"query": `query findOneVideo($slug: String) {
			findOneVideo(input: { slug: $slug }) {
				id
				title
				description
				releaseDate
			}
		}`,
		"variables": map[string]string{"slug": slug},
	}

	apiURL := "https://www.vixen.com/graphql"
	body, err := s.httpPostJSON(ctx, apiURL, gqlQuery)
	if err != nil {
		return nil, fmt.Errorf("Vixen scrape: %w", err)
	}

	var apiResp struct {
		Data struct {
			FindOneVideo struct {
				Description string `json:"description"`
				ReleaseDate string `json:"releaseDate"`
			} `json:"findOneVideo"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("Vixen: parsing response: %w", err)
	}

	video := apiResp.Data.FindOneVideo
	metadata := &GalleryMetadata{
		Provider:    "Vixen",
		SourceURL:   sourceURL,
		Description: video.Description,
	}

	if video.ReleaseDate != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", video.ReleaseDate); err == nil {
			metadata.ReleaseDate = parsed
		} else if len(video.ReleaseDate) >= 10 {
			if parsed, err := time.Parse("2006-01-02", video.ReleaseDate[:10]); err == nil {
				metadata.ReleaseDate = parsed
			}
		}
	}

	return metadata, nil
}

// ---------------------------------------------------------------------------
// VivThomas & WowGirls (HTML scraping with ?s= search)
// ---------------------------------------------------------------------------

func (s *GalleryMetadataService) searchVivThomas(ctx context.Context, query string) ([]GallerySearchResult, error) {
	return s.searchHTMLSite(ctx, "VivThomas", "https://www.vivthomas.com", query)
}

func (s *GalleryMetadataService) searchWowGirls(ctx context.Context, query string) ([]GallerySearchResult, error) {
	return s.searchHTMLSite(ctx, "WowGirls", "https://www.wowgirls.com", query)
}

func (s *GalleryMetadataService) searchHTMLSite(ctx context.Context, provider, baseURL, query string) ([]GallerySearchResult, error) {
	searchURL := fmt.Sprintf("%s/?s=%s", baseURL, url.QueryEscape(query))

	body, err := s.httpGet(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", provider, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("%s: parsing HTML: %w", provider, err)
	}

	galleryPatterns := []string{"/gallery/", "/photo-", "/series/", "/set/", "/update/", "/photos/"}

	var results []GallerySearchResult
	doc.Find("a").Each(func(i int, sel *goquery.Selection) {
		if i >= 50 {
			return
		}
		href, _ := sel.Attr("href")
		if href == "" {
			return
		}
		lower := strings.ToLower(href)

		isGallery := false
		for _, pat := range galleryPatterns {
			if strings.Contains(lower, pat) {
				isGallery = true
				break
			}
		}
		if !isGallery {
			return
		}

		if !strings.HasPrefix(href, "http") {
			if strings.HasPrefix(href, "/") {
				href = baseURL + href
			} else {
				href = baseURL + "/" + href
			}
		}

		text := strings.TrimSpace(sel.Text())
		thumb := ""
		if img := sel.Find("img"); img.Length() > 0 {
			if src, ok := img.Attr("src"); ok {
				thumb = src
			}
		}
		if text == "" {
			if img := sel.Find("img"); img.Length() > 0 {
				if alt, ok := img.Attr("alt"); ok {
					text = strings.TrimSpace(alt)
				}
			}
		}
		if text == "" {
			text = href
		}

		results = append(results, GallerySearchResult{
			Provider:  provider,
			Title:     text,
			URL:       href,
			Thumbnail: thumb,
		})
	})

	return results, nil
}

// scrapeHTMLGallery scrapes metadata from an HTML gallery page using og: meta tags.
// Works for VivThomas, WowGirls, and MPLStudios.
func (s *GalleryMetadataService) scrapeHTMLGallery(ctx context.Context, provider, sourceURL string) (*GalleryMetadata, error) {
	body, err := s.httpGet(ctx, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("%s scrape: %w", provider, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("%s: parsing HTML: %w", provider, err)
	}

	metadata := &GalleryMetadata{Provider: provider, SourceURL: sourceURL}

	if title, ok := doc.Find("meta[property='og:title']").Attr("content"); ok {
		metadata.Description = title
	} else {
		metadata.Description = strings.TrimSpace(doc.Find("h1, .title, .entry-title").First().Text())
	}

	if desc, ok := doc.Find("meta[property='og:description']").Attr("content"); ok {
		metadata.Description = desc
	}

	if thumb, ok := doc.Find("meta[property='og:image']").Attr("content"); ok {
		metadata.ThumbnailURL = thumb
	}

	// Try to extract release date from meta tag or visible elements.
	if dateStr, ok := doc.Find("meta[name='date']").Attr("content"); ok {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			metadata.ReleaseDate = parsed
		}
	}
	if metadata.ReleaseDate.IsZero() {
		metadata.ReleaseDate = parseDateFromDoc(doc)
	}

	return metadata, nil
}

// ---------------------------------------------------------------------------
// MPLStudios (complex nested JSON + HTML fallback)
// ---------------------------------------------------------------------------

func (s *GalleryMetadataService) searchMPLStudios(ctx context.Context, query string) ([]GallerySearchResult, error) {
	const base = "https://www.mplstudios.com"

	// Primary: /searchFor/ endpoint returning nested JSON arrays.
	searchForURL := fmt.Sprintf("%s/searchFor/?value=%s", base, url.QueryEscape(query))

	body, err := s.httpGet(ctx, searchForURL)
	if err != nil {
		slog.Debug("MPLStudios searchFor failed", "error", err)
		return s.searchMPLStudiosFallback(ctx, query)
	}

	// Check for age gate.
	bodyLower := strings.ToLower(string(body))
	if strings.Contains(bodyLower, "age verification") || strings.Contains(bodyLower, "verify your age") {
		slog.Debug("MPLStudios appears age-gated")
		return nil, fmt.Errorf("MPLStudios: age-gated response")
	}

	// Try parsing as JSON array.
	var root any
	if err := json.Unmarshal(body, &root); err == nil {
		// Direct gallery parsing from searchFor response (rootArr[1] contains galleries).
		if results := s.parseMPLSearchForGalleries(root, base); len(results) > 0 {
			return capResults(results, maxResultsPerProvider), nil
		}

		// Person match — fetch their page and extract galleries.
		// Only include galleries with title relevance to the query.
		if href, _, ok := findBestPersonFromSearchFor(root, query); ok {
			if !strings.HasPrefix(href, "http") {
				if strings.HasPrefix(href, "/") {
					href = base + href
				} else {
					href = base + "/" + href
				}
			}
			if results, err := s.parseMPLPersonPage(ctx, href, base); err == nil && len(results) > 0 {
				filtered := filterByTitleRelevance(results, query)
				if len(filtered) > 0 {
					return capResults(filtered, maxResultsPerProvider), nil
				}
				// If no title-relevant results, return a small sample of the person's galleries.
				return capResults(results, 5), nil
			}
		}
	}

	// Try extracting JSON from HTML wrapper.
	if cleaned, err := extractJSONFromHTML(body); err == nil {
		cleanedStr := strings.ReplaceAll(string(cleaned), "\\/", "/")
		var root2 any
		if err := json.Unmarshal([]byte(cleanedStr), &root2); err == nil {
			if href, _, ok := findBestPersonFromSearchFor(root2, query); ok {
				if !strings.HasPrefix(href, "http") {
					href = base + href
				}
				if results, err := s.parseMPLPersonPage(ctx, href, base); err == nil && len(results) > 0 {
					filtered := filterByTitleRelevance(results, query)
					if len(filtered) > 0 {
						return capResults(filtered, maxResultsPerProvider), nil
					}
					return capResults(results, 5), nil
				}
			}
		}
	}

	return s.searchMPLStudiosFallback(ctx, query)
}

func (s *GalleryMetadataService) searchMPLStudiosFallback(ctx context.Context, query string) ([]GallerySearchResult, error) {
	const base = "https://www.mplstudios.com"

	// Try alternative endpoints.
	candidates := []string{
		fmt.Sprintf("%s/api/search?query=%s", base, url.QueryEscape(query)),
		fmt.Sprintf("%s/search?query=%s", base, url.QueryEscape(query)),
		fmt.Sprintf("%s/galleries?search=%s", base, url.QueryEscape(query)),
	}

	for _, apiURL := range candidates {
		body, err := s.httpGet(ctx, apiURL)
		if err != nil {
			continue
		}

		// Try parsing as HTML and find gallery links.
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			continue
		}

		var results []GallerySearchResult
		doc.Find("a[href*='/gallery/'], a[href*='/galleries/']").Each(func(i int, sel *goquery.Selection) {
			href, _ := sel.Attr("href")
			text := strings.TrimSpace(sel.Text())
			thumb := ""
			if img := sel.Find("img"); img.Length() > 0 {
				if src, ok := img.Attr("src"); ok {
					thumb = src
				}
			}
			if href != "" {
				if !strings.HasPrefix(href, "http") {
					href = base + href
				}
				results = append(results, GallerySearchResult{
					Provider:  "MPLStudios",
					Title:     text,
					URL:       href,
					Thumbnail: thumb,
				})
			}
		})

		if len(results) > 0 {
			return capResults(results, maxResultsPerProvider), nil
		}
	}

	return nil, fmt.Errorf("no results from MPLStudios search")
}

func (s *GalleryMetadataService) parseMPLSearchForGalleries(root any, base string) []GallerySearchResult {
	rootArr, ok := root.([]any)
	if !ok || len(rootArr) < 2 {
		return nil
	}
	galleryGroups, ok := rootArr[1].([]any)
	if !ok || len(galleryGroups) == 0 {
		return nil
	}
	firstGroup, ok := galleryGroups[0].([]any)
	if !ok {
		return nil
	}

	var results []GallerySearchResult
	for _, entry := range firstGroup {
		entArr, ok := entry.([]any)
		if !ok || len(entArr) < 3 {
			continue
		}
		titleRaw := fmt.Sprintf("%v", entArr[2])
		urlRaw := fmt.Sprintf("%v", entArr[1])
		thumbRaw := fmt.Sprintf("%v", entArr[0])

		titleClean := stripHTML(titleRaw)
		urlClean := urlRaw
		if !strings.HasPrefix(urlClean, "http") {
			if strings.HasPrefix(urlClean, "/") {
				urlClean = base + urlClean
			} else {
				urlClean = base + "/" + urlClean
			}
		}

		results = append(results, GallerySearchResult{
			Provider:  "MPLStudios",
			Title:     titleClean,
			URL:       urlClean,
			Thumbnail: thumbRaw,
		})
	}

	return results
}

func (s *GalleryMetadataService) parseMPLPersonPage(ctx context.Context, personURL, base string) ([]GallerySearchResult, error) {
	body, err := s.httpGet(ctx, personURL)
	if err != nil {
		return nil, fmt.Errorf("fetching MPLStudios person page: %w", err)
	}

	pageStr := string(body)

	// Try embedded JS array: var result = [[...]]
	re := regexp.MustCompile(`(?s)(?:var|let|const)\s+result\s*=\s*(\[\s*\[.*?\]\s*\])\s*;`)
	match := re.FindStringSubmatch(pageStr)
	var arrayText string
	if len(match) >= 2 {
		arrayText = match[1]
	} else {
		re2 := regexp.MustCompile(`(?s)(\[\s*\[.*?\]\s*\])`)
		m2 := re2.FindStringSubmatch(pageStr)
		if len(m2) >= 2 {
			arrayText = m2[1]
		}
	}

	if arrayText != "" {
		cleaned := strings.ReplaceAll(arrayText, "\\/", "/")
		var parsed any
		if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
			if results := s.parseMPLSearchForGalleries(parsed, base); len(results) > 0 {
				return results, nil
			}
		}
	}

	// DOM fallback.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageStr))
	if err != nil {
		return nil, fmt.Errorf("parsing MPLStudios person page: %w", err)
	}

	var results []GallerySearchResult
	doc.Find("a[href*='/gallery/'], a[href*='/galleries/']").Each(func(i int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		text := strings.TrimSpace(sel.Text())
		thumb := ""
		if img := sel.Find("img"); img.Length() > 0 {
			if src, ok := img.Attr("src"); ok {
				thumb = src
			}
		}
		if href != "" {
			if !strings.HasPrefix(href, "http") {
				href = base + href
			}
			results = append(results, GallerySearchResult{
				Provider:  "MPLStudios",
				Title:     text,
				URL:       href,
				Thumbnail: thumb,
			})
		}
	})

	return results, nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (s *GalleryMetadataService) httpGet(ctx context.Context, targetURL string) ([]byte, error) {
	client := s.getClient(targetURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building GET %q: %w", targetURL, err)
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %q: %w", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %q returned %d", targetURL, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *GalleryMetadataService) httpPostJSON(ctx context.Context, targetURL string, payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}

	client := s.getClient(targetURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(encoded)))
	if err != nil {
		return nil, fmt.Errorf("building POST %q: %w", targetURL, err)
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %q: %w", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %q returned %d", targetURL, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *GalleryMetadataService) algoliaPost(ctx context.Context, apiURL, body string) ([]byte, error) {
	client := s.getClient(apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building Algolia POST: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Algolia-Application-Id", algoliaAppID)
	req.Header.Set("X-Algolia-API-Key", algoliaAPIKey)
	req.Header.Set("Referer", "https://www.playboyplus.com/")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Algolia POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Algolia returned %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *GalleryMetadataService) algoliaGet(ctx context.Context, apiURL string) ([]byte, error) {
	client := s.getClient(apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building Algolia GET: %w", err)
	}
	req.Header.Set("X-Algolia-Application-Id", algoliaAppID)
	req.Header.Set("X-Algolia-API-Key", algoliaAPIKey)
	req.Header.Set("Referer", "https://www.playboyplus.com/")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Algolia GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Algolia returned %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ---------------------------------------------------------------------------
// URL / HTML parsing helpers
// ---------------------------------------------------------------------------

// parseMetArtGalleryURL extracts the gallery date and name from a MetArt-network URL.
// Expected format: .../gallery/{date}/{name} or fallback to last two path segments.
func parseMetArtGalleryURL(rawURL string) (date, name string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("URL path too short to extract gallery date/name")
	}

	for i, part := range parts {
		if part == "gallery" && i+2 < len(parts) {
			return parts[i+1], parts[i+2], nil
		}
	}

	// Fallback: last two segments.
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

// toAbsoluteURL ensures a path is absolute relative to the given base.
func toAbsoluteURL(base, path string) string {
	if strings.HasPrefix(path, "http") {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}

// parseDateFromDoc attempts to find a release date in common HTML elements.
func parseDateFromDoc(doc *goquery.Document) time.Time {
	dateText := strings.TrimSpace(doc.Find(".date, time, .posted-on, .entry-date, .publish-date").First().Text())
	if dateText == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02",
		"January 2, 2006",
		"Jan 2, 2006",
		"02/01/2006",
	} {
		if parsed, err := time.Parse(layout, dateText); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// stripHTML removes simple HTML tags and unescapes common entities.
func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	cleaned := re.ReplaceAllString(s, "")
	cleaned = html.UnescapeString(cleaned)
	return strings.TrimSpace(cleaned)
}

// extractJSONFromHTML attempts to extract a JSON chunk from an HTML wrapper.
func extractJSONFromHTML(body []byte) ([]byte, error) {
	s := string(body)

	// 1) <pre>...</pre>
	rePre := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)
	if m := rePre.FindStringSubmatch(s); len(m) >= 2 {
		return []byte(html.UnescapeString(m[1])), nil
	}

	// 2) JS assignment: var x = [...] or window.__DATA__ = {...}
	reAssign := regexp.MustCompile(`(?is)(?:var|let|const|window\.[\w_]+|window\['[\w_]+'\])\s*=\s*([\[{][\s\S]*?[\]}])\s*;`)
	if m := reAssign.FindStringSubmatch(s); len(m) >= 2 {
		return []byte(m[1]), nil
	}

	// 3) Fallback: find first '{' or '[' and return balanced chunk.
	startIdx := -1
	var open, close byte
	for i := 0; i < len(s); i++ {
		if s[i] == '{' || s[i] == '[' {
			startIdx = i
			if s[i] == '{' {
				open, close = '{', '}'
			} else {
				open, close = '[', ']'
			}
			break
		}
	}
	if startIdx == -1 {
		return nil, fmt.Errorf("no JSON start char found")
	}

	depth := 0
	inString := false
	esc := false
	for i := startIdx; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == open {
			depth++
		} else if c == close {
			depth--
			if depth == 0 {
				return []byte(s[startIdx : i+1]), nil
			}
		}
	}

	return nil, fmt.Errorf("unbalanced JSON")
}

// findBestPersonFromSearchFor inspects the nested array response from
// MPLStudios /searchFor/ and returns the href and name for the best
// matching person entry.
func findBestPersonFromSearchFor(root any, alias string) (href string, name string, ok bool) {
	aliasLower := strings.ToLower(alias)
	type candidate struct {
		href  string
		name  string
		score int
	}
	var candidates []candidate

	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case []any:
			if len(v) >= 3 {
				var strs []string
				for _, it := range v {
					if s, ok := it.(string); ok {
						strs = append(strs, s)
					}
				}
				if len(strs) >= 2 {
					var candidateHref, candidateName string
					for _, t := range strs {
						if strings.Contains(t, "/") && (strings.HasPrefix(t, "http") || strings.HasPrefix(t, "/")) {
							candidateHref = t
						} else if len(t) > 0 && (strings.Contains(t, " ") || unicode.IsLetter(rune(t[0]))) {
							candidateName = t
						}
					}
					if candidateHref != "" && candidateName != "" {
						score := 0
						lname := strings.ToLower(candidateName)
						if strings.EqualFold(candidateName, alias) ||
							strings.Contains(aliasLower, lname) ||
							strings.Contains(lname, aliasLower) {
							score += 10
						}
						if len(aliasLower) >= 2 && len(lname) >= 2 &&
							aliasLower[:2] == lname[:2] &&
							absInt(len(aliasLower)-len(lname)) <= 3 {
							score += 5
						}
						candidates = append(candidates, candidate{candidateHref, candidateName, score})
					}
				}
			}
			for _, it := range v {
				walk(it)
			}
		case map[string]any:
			if n, ok := v["name"].(string); ok {
				if u, ok2 := v["url"].(string); ok2 {
					score := 0
					lname := strings.ToLower(n)
					if strings.Contains(lname, aliasLower) || strings.EqualFold(n, alias) {
						score += 10
					}
					candidates = append(candidates, candidate{u, n, score})
				}
			}
			for _, it := range v {
				walk(it)
			}
		}
	}
	walk(root)

	bestScore := 0
	bestIdx := -1
	for i, c := range candidates {
		if c.score > bestScore {
			bestScore = c.score
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return candidates[bestIdx].href, candidates[bestIdx].name, true
	}
	return "", "", false
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// capResults returns at most n results from the slice.
func capResults(results []GallerySearchResult, n int) []GallerySearchResult {
	if len(results) <= n {
		return results
	}
	return results[:n]
}

// filterByTitleRelevance keeps only results whose title shares at least one
// significant word (3+ chars) with the query. This prevents returning every
// gallery from a person page when searching for a specific gallery name.
func filterByTitleRelevance(results []GallerySearchResult, query string) []GallerySearchResult {
	queryWords := significantWords(query)
	if len(queryWords) == 0 {
		return results
	}

	var filtered []GallerySearchResult
	for _, r := range results {
		titleWords := significantWords(r.Title)
		for _, qw := range queryWords {
			for _, tw := range titleWords {
				if strings.EqualFold(qw, tw) {
					filtered = append(filtered, r)
					goto next
				}
			}
		}
	next:
	}
	return filtered
}

// significantWords splits s into lowercase words of 3+ characters, filtering
// out common noise words.
func significantWords(s string) []string {
	noise := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"that": true, "this": true, "are": true, "was": true, "has": true,
	}
	var words []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		// Strip non-alphanumeric edges.
		w = strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if len(w) >= 3 && !noise[w] {
			words = append(words, w)
		}
	}
	return words
}
