package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const metartBaseURL = "https://www.metart.com/api"

// MetArt queries the MetArt API for galleries and performer metadata.
// It also covers MetartX (same API, different base URL).
type MetArt struct {
	baseClient
	baseURL string
}

// NewMetArt creates a MetArt API client for metart.com.
func NewMetArt(client *http.Client, userAgent string) *MetArt {
	return &MetArt{
		baseClient: newBaseClient(client, userAgent),
		baseURL:    metartBaseURL,
	}
}

// NewMetArtX creates a MetArt API client for metartx.com.
func NewMetArtX(client *http.Client, userAgent string) *MetArt {
	return &MetArt{
		baseClient: newBaseClient(client, userAgent),
		baseURL:    "https://www.metartx.com/api",
	}
}

// MetArt JSON response shapes.
type metartModelResponse struct {
	UUID        string  `json:"UUID"`
	Name        string  `json:"name"`
	Country     *string `json:"country"`
	BirthDate   *string `json:"birthDate"`
	HairColor   *string `json:"hairColor"`
	EyeColor    *string `json:"eyeColor"`
	Height      *int    `json:"height"` // cm
	Weight      *int    `json:"weight"` // kg
	Bust        *string `json:"bust"`
	Waist       *string `json:"waist"`
	Hips        *string `json:"hips"`
	HeadshotURL *string `json:"headshotURL"`
}

type metartGalleryResponse struct {
	UUID         string   `json:"UUID"`
	Name         string   `json:"name"`
	URL          string   `json:"siteURL"`
	CoverURL     *string  `json:"coverURL"`
	Date         *string  `json:"publishedAt"`
	Models       []string `json:"modelNames"`
	ThumbnailURL *string  `json:"thumbnailURL"`
}

// SearchModel searches for a model by name on MetArt.
func (m *MetArt) SearchModel(ctx context.Context, name string) (*PersonInfo, error) {
	u := fmt.Sprintf("%s/model?name=%s", m.baseURL, url.QueryEscape(name))

	body, err := m.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("metart: model search %q: %w", name, err)
	}

	var model metartModelResponse
	if err := json.Unmarshal([]byte(body), &model); err != nil {
		return nil, fmt.Errorf("metart: decoding model response: %w", err)
	}

	info := mapMetArtModel(model)
	return &info, nil
}

// GetModel fetches a model by UUID.
func (m *MetArt) GetModel(ctx context.Context, uuid string) (*PersonInfo, error) {
	u := fmt.Sprintf("%s/model/%s", m.baseURL, url.PathEscape(uuid))

	body, err := m.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("metart: get model %q: %w", uuid, err)
	}

	var model metartModelResponse
	if err := json.Unmarshal([]byte(body), &model); err != nil {
		return nil, fmt.Errorf("metart: decoding model response: %w", err)
	}

	info := mapMetArtModel(model)
	return &info, nil
}

// SearchGalleries returns galleries matching a model name.
func (m *MetArt) SearchGalleries(ctx context.Context, modelName string) ([]GalleryInfo, error) {
	u := fmt.Sprintf("%s/galleries?model=%s", m.baseURL, url.QueryEscape(modelName))

	body, err := m.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("metart: gallery search %q: %w", modelName, err)
	}

	var galleries []metartGalleryResponse
	if err := json.Unmarshal([]byte(body), &galleries); err != nil {
		return nil, fmt.Errorf("metart: decoding galleries: %w", err)
	}

	results := make([]GalleryInfo, 0, len(galleries))
	for _, g := range galleries {
		results = append(results, mapMetArtGallery(g, m.providerName()))
	}
	return results, nil
}

func (m *MetArt) providerName() string {
	if m.baseURL == metartBaseURL {
		return "metart"
	}
	return "metartx"
}

func mapMetArtModel(model metartModelResponse) PersonInfo {
	info := PersonInfo{
		Name:       model.Name,
		ExternalID: ptrStr(model.UUID),
	}

	if model.BirthDate != nil {
		info.BirthDate = parseDate(*model.BirthDate)
	}
	info.Nationality = model.Country
	info.HairColor = model.HairColor
	info.EyeColor = model.EyeColor

	if model.Height != nil {
		h := fmt.Sprintf("%dcm", *model.Height)
		info.Height = &h
	}
	if model.Weight != nil {
		w := fmt.Sprintf("%dkg", *model.Weight)
		info.Weight = &w
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

	info.ImageURL = model.HeadshotURL
	if info.ImageURL != nil {
		info.ImageURLs = []string{*info.ImageURL}
	}

	return info
}

func mapMetArtGallery(g metartGalleryResponse, provider string) GalleryInfo {
	gi := GalleryInfo{
		Title:      g.Name,
		URL:        g.URL,
		Provider:   provider,
		ProviderID: g.UUID,
		Performers: g.Models,
	}
	if g.CoverURL != nil {
		gi.ThumbnailURL = g.CoverURL
	} else if g.ThumbnailURL != nil {
		gi.ThumbnailURL = g.ThumbnailURL
	}
	if g.Date != nil {
		gi.Date = parseDate(*g.Date)
	}
	return gi
}

// deref returns the string value of a *string, or "" if nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
