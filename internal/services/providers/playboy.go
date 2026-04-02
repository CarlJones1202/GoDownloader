package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const playboyCMSURL = "https://www.playboy.com/api"

// Playboy queries the Playboy API for galleries and performer metadata.
type Playboy struct {
	baseClient
}

// NewPlayboy creates a Playboy API client.
func NewPlayboy(client *http.Client, userAgent string) *Playboy {
	return &Playboy{baseClient: newBaseClient(client, userAgent)}
}

// Playboy API response shapes.
type playboyModelResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Nationality *string `json:"nationality"`
	BirthDate   *string `json:"dateOfBirth"`
	HairColor   *string `json:"hairColor"`
	EyeColor    *string `json:"eyeColor"`
	Height      *string `json:"height"`
	Weight      *string `json:"weight"`
	Bust        *string `json:"bust"`
	Waist       *string `json:"waist"`
	Hips        *string `json:"hips"`
	ImageURL    *string `json:"imageUrl"`
	Bio         *string `json:"biography"`
}

type playboyGalleryResponse struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	URL          string   `json:"url"`
	ThumbnailURL *string  `json:"thumbnailUrl"`
	Date         *string  `json:"publishedAt"`
	Models       []string `json:"modelNames"`
}

// SearchModel searches for a model by name on Playboy.
func (p *Playboy) SearchModel(ctx context.Context, name string) (*PersonInfo, error) {
	u := fmt.Sprintf("%s/playmates?search=%s", playboyCMSURL, url.QueryEscape(name))

	body, err := p.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("playboy: model search %q: %w", name, err)
	}

	var models []playboyModelResponse
	if err := json.Unmarshal([]byte(body), &models); err != nil {
		return nil, fmt.Errorf("playboy: decoding model search: %w", err)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("playboy: no results for %q", name)
	}

	info := mapPlayboyModel(models[0])
	return &info, nil
}

// GetModel fetches a model by Playboy slug.
func (p *Playboy) GetModel(ctx context.Context, slug string) (*PersonInfo, error) {
	u := fmt.Sprintf("%s/playmates/%s", playboyCMSURL, url.PathEscape(slug))

	body, err := p.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("playboy: get model %q: %w", slug, err)
	}

	var model playboyModelResponse
	if err := json.Unmarshal([]byte(body), &model); err != nil {
		return nil, fmt.Errorf("playboy: decoding model: %w", err)
	}

	info := mapPlayboyModel(model)
	return &info, nil
}

// SearchGalleries returns galleries matching a model name.
func (p *Playboy) SearchGalleries(ctx context.Context, modelName string) ([]GalleryInfo, error) {
	u := fmt.Sprintf("%s/galleries?model=%s", playboyCMSURL, url.QueryEscape(modelName))

	body, err := p.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("playboy: gallery search %q: %w", modelName, err)
	}

	var galleries []playboyGalleryResponse
	if err := json.Unmarshal([]byte(body), &galleries); err != nil {
		return nil, fmt.Errorf("playboy: decoding galleries: %w", err)
	}

	results := make([]GalleryInfo, 0, len(galleries))
	for _, g := range galleries {
		results = append(results, mapPlayboyGallery(g))
	}
	return results, nil
}

func mapPlayboyModel(model playboyModelResponse) PersonInfo {
	info := PersonInfo{
		Name:       model.Name,
		ExternalID: ptrStr(model.Slug),
		Biography:  model.Bio,
	}

	if model.BirthDate != nil {
		info.BirthDate = parseDate(*model.BirthDate)
	}
	info.Nationality = model.Nationality
	info.HairColor = model.HairColor
	info.EyeColor = model.EyeColor
	info.Height = model.Height
	info.Weight = model.Weight
	info.ImageURL = model.ImageURL
	if info.ImageURL != nil {
		info.ImageURLs = []string{*info.ImageURL}
	}

	// Build measurements from individual fields.
	if model.Bust != nil || model.Waist != nil || model.Hips != nil {
		parts := []string{
			deref(model.Bust),
			deref(model.Waist),
			deref(model.Hips),
		}
		m := joinNonEmpty(parts, "-")
		info.Measurements = ptrStr(m)
	}

	return info
}

func mapPlayboyGallery(g playboyGalleryResponse) GalleryInfo {
	gi := GalleryInfo{
		Title:      g.Title,
		URL:        g.URL,
		Provider:   "playboy",
		ProviderID: g.Slug,
		Performers: g.Models,
	}
	gi.ThumbnailURL = g.ThumbnailURL
	if g.Date != nil {
		gi.Date = parseDate(*g.Date)
	}
	return gi
}
