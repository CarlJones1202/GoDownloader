// Package handlers — people HTTP handlers.
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/personphoto"
	"github.com/carlj/godownload/internal/services/providers"
	"github.com/gin-gonic/gin"
)

// PeopleHandler handles HTTP requests for the /api/v1/people resource.
type PeopleHandler struct {
	db              *database.DB
	linker          *linker.AutoLinker
	enricher        *providers.Enricher
	photoDownloader *personphoto.Downloader
}

// NewPeopleHandler creates a PeopleHandler.
func NewPeopleHandler(db *database.DB, al *linker.AutoLinker, enricher *providers.Enricher, photoDownloader *personphoto.Downloader) *PeopleHandler {
	return &PeopleHandler{db: db, linker: al, enricher: enricher, photoDownloader: photoDownloader}
}

// RegisterRoutes registers all people routes on the given group.
func (h *PeopleHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.POST("/bulk/enrich", h.bulkEnrich)
	rg.POST("/bulk/merge", h.bulkMerge)
	rg.DELETE("/bulk", h.bulkDelete)
	rg.GET("/providers", h.listProviders)
	rg.GET("/:id", h.get)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
	rg.GET("/:id/galleries", h.listGalleries)
	rg.POST("/:id/link-gallery/:galleryId", h.linkGallery)
	rg.POST("/:id/unlink-gallery/:galleryId", h.unlinkGallery)
	rg.GET("/:id/identifiers", h.listIdentifiers)
	rg.POST("/:id/identifiers", h.upsertIdentifier)
	rg.POST("/:id/enrich", h.enrich)
	rg.GET("/:id/search", h.searchProviders)
	rg.POST("/:id/identify", h.identify)
}

type createPersonRequest struct {
	Name        string  `json:"name"        binding:"required"`
	Aliases     *string `json:"aliases"`
	BirthDate   *string `json:"birth_date"` // ISO 8601 date
	Nationality *string `json:"nationality"`
}

type updatePersonRequest struct {
	Name         *string `json:"name"`
	Aliases      *string `json:"aliases"`
	BirthDate    *string `json:"birth_date"`
	Nationality  *string `json:"nationality"`
	Ethnicity    *string `json:"ethnicity"`
	HairColor    *string `json:"hair_color"`
	EyeColor     *string `json:"eye_color"`
	Height       *string `json:"height"`
	Weight       *string `json:"weight"`
	Measurements *string `json:"measurements"`
	Tattoos      *string `json:"tattoos"`
	Piercings    *string `json:"piercings"`
	Biography    *string `json:"biography"`
}

type upsertIdentifierRequest struct {
	Provider   string `json:"provider"    binding:"required"`
	ExternalID string `json:"external_id" binding:"required"`
}

func (h *PeopleHandler) list(c *gin.Context) {
	limit, offset := paginationParams(c)

	f := database.PeopleFilter{
		Limit:  limit,
		Offset: offset,
	}
	if v := c.Query("search"); v != "" {
		f.Search = &v
	}

	people, err := h.db.ListPeople(c.Request.Context(), f)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, people)
}

func (h *PeopleHandler) get(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	p, err := h.db.GetPerson(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, p)
}

func (h *PeopleHandler) create(c *gin.Context) {
	var req createPersonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	p := &models.Person{
		Name:        req.Name,
		Aliases:     req.Aliases,
		Nationality: req.Nationality,
	}

	if err := h.db.CreatePerson(c.Request.Context(), p); err != nil {
		handleDBError(c, err)
		return
	}

	// Auto-link to matching galleries in the background.
	go h.linker.LinkPerson(c.Request.Context(), p) //nolint:errcheck

	respondCreated(c, p)
}

func (h *PeopleHandler) update(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	p, err := h.db.GetPerson(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	var req updatePersonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Aliases != nil {
		p.Aliases = req.Aliases
	}
	if req.Nationality != nil {
		p.Nationality = req.Nationality
	}
	if req.Ethnicity != nil {
		p.Ethnicity = req.Ethnicity
	}
	if req.HairColor != nil {
		p.HairColor = req.HairColor
	}
	if req.EyeColor != nil {
		p.EyeColor = req.EyeColor
	}
	if req.Height != nil {
		p.Height = req.Height
	}
	if req.Weight != nil {
		p.Weight = req.Weight
	}
	if req.Measurements != nil {
		p.Measurements = req.Measurements
	}
	if req.Tattoos != nil {
		p.Tattoos = req.Tattoos
	}
	if req.Piercings != nil {
		p.Piercings = req.Piercings
	}
	if req.Biography != nil {
		p.Biography = req.Biography
	}

	if err := h.db.UpdatePerson(c.Request.Context(), p); err != nil {
		handleDBError(c, err)
		return
	}

	// Re-run auto-link in case name or aliases changed.
	go h.linker.LinkPerson(c.Request.Context(), p) //nolint:errcheck

	respondOK(c, p)
}

func (h *PeopleHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.db.DeletePerson(c.Request.Context(), id); err != nil {
		handleDBError(c, err)
		return
	}
	respondNoContent(c)
}

func (h *PeopleHandler) listGalleries(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	limit, offset := paginationParams(c)

	galleries, err := h.db.ListPersonGalleries(c.Request.Context(), id, limit, offset)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, galleries)
}

func (h *PeopleHandler) linkGallery(c *gin.Context) {
	personID, ok := parseIDParam(c)
	if !ok {
		return
	}
	galleryID, ok := parseIntParam(c, "galleryId")
	if !ok {
		return
	}

	if err := h.db.LinkGallery(c.Request.Context(), personID, galleryID); err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{"message": "linked"})
}

func (h *PeopleHandler) unlinkGallery(c *gin.Context) {
	personID, ok := parseIDParam(c)
	if !ok {
		return
	}
	galleryID, ok := parseIntParam(c, "galleryId")
	if !ok {
		return
	}

	if err := h.db.UnlinkGallery(c.Request.Context(), personID, galleryID); err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, gin.H{"message": "unlinked"})
}

func (h *PeopleHandler) listIdentifiers(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	ids, err := h.db.ListPersonIdentifiers(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}
	respondOK(c, ids)
}

func (h *PeopleHandler) upsertIdentifier(c *gin.Context) {
	personID, ok := parseIDParam(c)
	if !ok {
		return
	}

	var req upsertIdentifierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	pid := &models.PersonIdentifier{
		PersonID:   personID,
		Provider:   req.Provider,
		ExternalID: req.ExternalID,
	}

	if err := h.db.UpsertPersonIdentifier(c.Request.Context(), pid); err != nil {
		handleDBError(c, err)
		return
	}
	respondCreated(c, pid)
}

// enrich fetches metadata from all external providers for the person and
// optionally merges it into the local record.
//
// Query params:
//   - provider: limit to a single provider (stashdb, freeones, babepedia, metart, metartx, playboy)
//   - apply: if "true", merge the best metadata into the person record
func (h *PeopleHandler) enrich(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	p, err := h.db.GetPerson(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	providerName := c.Query("provider")
	applyChanges := c.Query("apply") == "true"

	if providerName != "" {
		// Single-provider lookup.
		info, lookupErr := h.enricher.LookupProvider(c.Request.Context(), providerName, p.Name)
		if lookupErr != nil {
			respondError(c, http.StatusBadGateway, lookupErr.Error())
			return
		}

		if applyChanges {
			h.applyPersonInfo(c, p, info, providerName)
			return
		}

		respondOK(c, gin.H{
			"provider": providerName,
			"person":   info,
		})
		return
	}

	// All-provider lookup.
	result := h.enricher.LookupPerson(c.Request.Context(), p.Name)

	if applyChanges {
		h.applyPersonInfo(c, p, &result.Merged, "merged")
		return
	}

	respondOK(c, result)
}

// applyPersonInfo merges provider metadata into the person record, saves
// the external ID as an identifier, downloads photos, and returns the updated person.
func (h *PeopleHandler) applyPersonInfo(c *gin.Context, p *models.Person, info *providers.PersonInfo, providerName string) {
	h.mergePersonFields(p, info)

	// Download provider photos and store local paths.
	if h.photoDownloader != nil && len(info.ImageURLs) > 0 {
		localPaths := h.photoDownloader.DownloadAll(c.Request.Context(), info.ImageURLs, p.ID)
		if len(localPaths) > 0 {
			// Merge with any existing photos.
			existing := decodePhotoPaths(p.Photos)
			merged := mergePhotoPaths(existing, localPaths)
			encoded, err := json.Marshal(merged)
			if err == nil {
				s := string(encoded)
				p.Photos = &s
			}
		}
	}

	if err := h.db.UpdatePerson(c.Request.Context(), p); err != nil {
		handleDBError(c, err)
		return
	}

	// Store the external ID as a person identifier when available.
	if info.ExternalID != nil && providerName != "merged" {
		pid := &models.PersonIdentifier{
			PersonID:   p.ID,
			Provider:   providerName,
			ExternalID: *info.ExternalID,
		}
		// Best effort — don't fail the request if this errors.
		_ = h.db.UpsertPersonIdentifier(c.Request.Context(), pid)
	}

	respondOK(c, p)
}

// bulkEnrichRequest holds the request body for bulk enrichment.
type bulkEnrichRequest struct {
	PersonIDs []int64 `json:"person_ids" binding:"required"`
	Provider  string  `json:"provider"` // optional: limit to single provider
	Apply     bool    `json:"apply"`    // if true, merge metadata into person records
}

// bulkEnrich triggers enrichment for multiple people at once.
func (h *PeopleHandler) bulkEnrich(c *gin.Context) {
	var req bulkEnrichRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()
	enriched := 0
	failed := 0

	for _, pid := range req.PersonIDs {
		p, err := h.db.GetPerson(ctx, pid)
		if err != nil {
			slog.Warn("bulk enrich: skip person", "person_id", pid, "error", err)
			failed++
			continue
		}

		if req.Provider != "" {
			info, lookupErr := h.enricher.LookupProvider(ctx, req.Provider, p.Name)
			if lookupErr != nil {
				slog.Warn("bulk enrich: lookup failed", "person_id", pid, "provider", req.Provider, "error", lookupErr)
				failed++
				continue
			}
			if req.Apply {
				h.applyPersonInfoQuiet(ctx, p, info, req.Provider)
			}
		} else {
			result := h.enricher.LookupPerson(ctx, p.Name)
			if req.Apply {
				h.applyPersonInfoQuiet(ctx, p, &result.Merged, "merged")
			}
		}
		enriched++
	}

	respondOK(c, gin.H{
		"message":   "bulk enrichment complete",
		"enriched":  enriched,
		"failed":    failed,
		"requested": len(req.PersonIDs),
	})
}

// applyPersonInfoQuiet merges provider metadata into the person record without
// writing an HTTP response. Used for bulk operations.
func (h *PeopleHandler) applyPersonInfoQuiet(ctx context.Context, p *models.Person, info *providers.PersonInfo, providerName string) {
	h.mergePersonFields(p, info)

	// Download provider photos and store local paths.
	if h.photoDownloader != nil && len(info.ImageURLs) > 0 {
		localPaths := h.photoDownloader.DownloadAll(ctx, info.ImageURLs, p.ID)
		if len(localPaths) > 0 {
			existing := decodePhotoPaths(p.Photos)
			merged := mergePhotoPaths(existing, localPaths)
			encoded, err := json.Marshal(merged)
			if err == nil {
				s := string(encoded)
				p.Photos = &s
			}
		}
	}

	if err := h.db.UpdatePerson(ctx, p); err != nil {
		slog.Error("bulk enrich: update person", "person_id", p.ID, "error", err)
		return
	}

	if info.ExternalID != nil && providerName != "merged" {
		pid := &models.PersonIdentifier{
			PersonID:   p.ID,
			Provider:   providerName,
			ExternalID: *info.ExternalID,
		}
		_ = h.db.UpsertPersonIdentifier(ctx, pid)
	}
}

// bulkMergeRequest holds the request body for merging people.
type bulkMergeRequest struct {
	KeepID   int64   `json:"keep_id"   binding:"required"`
	MergeIDs []int64 `json:"merge_ids" binding:"required"`
}

// bulkMerge merges multiple people into a single keeper.
func (h *PeopleHandler) bulkMerge(c *gin.Context) {
	var req bulkMergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.db.MergePeople(c.Request.Context(), req.KeepID, req.MergeIDs); err != nil {
		handleDBError(c, err)
		return
	}

	// Return the updated keeper.
	keeper, err := h.db.GetPerson(c.Request.Context(), req.KeepID)
	if err != nil {
		handleDBError(c, err)
		return
	}

	respondOK(c, gin.H{
		"message": "merge complete",
		"person":  keeper,
		"merged":  len(req.MergeIDs),
	})
}

// bulkDeleteRequest holds the request body for bulk deletion.
type bulkDeleteRequest struct {
	PersonIDs []int64 `json:"person_ids" binding:"required"`
}

// bulkDelete deletes multiple people at once.
func (h *PeopleHandler) bulkDelete(c *gin.Context) {
	var req bulkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	deleted, err := h.db.BulkDeletePeople(c.Request.Context(), req.PersonIDs)
	if err != nil {
		handleDBError(c, err)
		return
	}

	respondOK(c, gin.H{
		"message":   "bulk delete complete",
		"requested": len(req.PersonIDs),
		"deleted":   deleted,
	})
}

// listProviders returns the available external metadata providers.
func (h *PeopleHandler) listProviders(c *gin.Context) {
	respondOK(c, h.enricher.ListProviders())
}

// searchProviders searches external providers for a person by name and returns
// all matching candidates (not just the best match).
//
// Query params:
//   - provider: which provider to search (required: stashdb, freeones, babepedia, metart, metartx, playboy)
//   - query: override search term (defaults to the person's name)
func (h *PeopleHandler) searchProviders(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	p, err := h.db.GetPerson(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	providerName := c.Query("provider")
	if providerName == "" {
		respondError(c, http.StatusBadRequest, "provider query parameter is required")
		return
	}

	query := c.Query("query")
	if query == "" {
		query = p.Name
	}

	results, searchErr := h.enricher.SearchProvider(c.Request.Context(), providerName, query)
	if searchErr != nil {
		respondError(c, http.StatusBadGateway, searchErr.Error())
		return
	}

	// Convert PersonInfo results to a JSON-friendly shape with string dates.
	type searchResult struct {
		Name         string   `json:"name"`
		Aliases      []string `json:"aliases,omitempty"`
		BirthDate    *string  `json:"birth_date,omitempty"`
		Nationality  *string  `json:"nationality,omitempty"`
		Ethnicity    *string  `json:"ethnicity,omitempty"`
		HairColor    *string  `json:"hair_color,omitempty"`
		EyeColor     *string  `json:"eye_color,omitempty"`
		Height       *string  `json:"height,omitempty"`
		Weight       *string  `json:"weight,omitempty"`
		Measurements *string  `json:"measurements,omitempty"`
		Tattoos      *string  `json:"tattoos,omitempty"`
		Piercings    *string  `json:"piercings,omitempty"`
		Biography    *string  `json:"biography,omitempty"`
		ImageURL     *string  `json:"image_url,omitempty"`
		ImageURLs    []string `json:"image_urls,omitempty"`
		ExternalID   *string  `json:"external_id,omitempty"`
	}

	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		sr := searchResult{
			Name:         r.Name,
			Aliases:      r.Aliases,
			Nationality:  r.Nationality,
			Ethnicity:    r.Ethnicity,
			HairColor:    r.HairColor,
			EyeColor:     r.EyeColor,
			Height:       r.Height,
			Weight:       r.Weight,
			Measurements: r.Measurements,
			Tattoos:      r.Tattoos,
			Piercings:    r.Piercings,
			Biography:    r.Biography,
			ImageURL:     r.ImageURL,
			ImageURLs:    r.ImageURLs,
			ExternalID:   r.ExternalID,
		}
		if r.BirthDate != nil {
			bd := r.BirthDate.Format("2006-01-02")
			sr.BirthDate = &bd
		}
		out = append(out, sr)
	}

	respondOK(c, gin.H{
		"provider": providerName,
		"query":    query,
		"results":  out,
	})
}

// identifyRequest is the body for POST /people/:id/identify.
type identifyRequest struct {
	Provider   string `json:"provider"    binding:"required"`
	ExternalID string `json:"external_id" binding:"required"`
	Apply      bool   `json:"apply"` // if true, also merge metadata into the person record
}

// identify links a person to a specific external provider result (by external ID),
// optionally applying the metadata to the person record.
func (h *PeopleHandler) identify(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	p, err := h.db.GetPerson(c.Request.Context(), id)
	if err != nil {
		handleDBError(c, err)
		return
	}

	var req identifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// Save the external identifier.
	pid := &models.PersonIdentifier{
		PersonID:   p.ID,
		Provider:   req.Provider,
		ExternalID: req.ExternalID,
	}
	if err := h.db.UpsertPersonIdentifier(c.Request.Context(), pid); err != nil {
		handleDBError(c, err)
		return
	}

	if !req.Apply {
		// Just save the identifier, don't fetch/apply metadata.
		respondOK(c, gin.H{
			"message":    "identifier saved",
			"person":     p,
			"identifier": pid,
		})
		return
	}

	// Fetch full details from the provider using the external ID and apply.
	info, fetchErr := h.enricher.GetByExternalID(c.Request.Context(), req.Provider, req.ExternalID)
	if fetchErr != nil {
		// The identifier was already saved — warn but don't fail.
		slog.Warn("identify: could not fetch details to apply",
			"person_id", p.ID, "provider", req.Provider, "external_id", req.ExternalID, "error", fetchErr)

		// Fall back: try searching by name and matching external ID from the results.
		results, searchErr := h.enricher.SearchProvider(c.Request.Context(), req.Provider, p.Name)
		if searchErr == nil {
			for _, r := range results {
				if r.ExternalID != nil && *r.ExternalID == req.ExternalID {
					info = &r
					break
				}
			}
		}
	}

	if info != nil {
		h.applyPersonInfo(c, p, info, req.Provider)
		return
	}

	// Identifier saved but couldn't fetch metadata to apply.
	respondOK(c, gin.H{
		"message":    "identifier saved (metadata fetch failed)",
		"person":     p,
		"identifier": pid,
	})
}

// mergePersonFields applies all metadata fields from a PersonInfo into the
// person record, only filling in fields that are currently nil/empty.
func (h *PeopleHandler) mergePersonFields(p *models.Person, info *providers.PersonInfo) {
	if info.Aliases != nil && p.Aliases == nil {
		joined := strings.Join(info.Aliases, ", ")
		p.Aliases = &joined
	}
	if info.BirthDate != nil && p.BirthDate == nil {
		p.BirthDate = info.BirthDate
	}
	if info.Nationality != nil && p.Nationality == nil {
		p.Nationality = info.Nationality
	}
	if info.Ethnicity != nil && p.Ethnicity == nil {
		p.Ethnicity = info.Ethnicity
	}
	if info.HairColor != nil && p.HairColor == nil {
		p.HairColor = info.HairColor
	}
	if info.EyeColor != nil && p.EyeColor == nil {
		p.EyeColor = info.EyeColor
	}
	if info.Height != nil && p.Height == nil {
		p.Height = info.Height
	}
	if info.Weight != nil && p.Weight == nil {
		p.Weight = info.Weight
	}
	if info.Measurements != nil && p.Measurements == nil {
		p.Measurements = info.Measurements
	}
	if info.Tattoos != nil && p.Tattoos == nil {
		p.Tattoos = info.Tattoos
	}
	if info.Piercings != nil && p.Piercings == nil {
		p.Piercings = info.Piercings
	}
	if info.Biography != nil && p.Biography == nil {
		p.Biography = info.Biography
	}
}

// decodePhotoPaths decodes a JSON array of photo paths from the person's Photos field.
func decodePhotoPaths(photos *string) []string {
	if photos == nil || *photos == "" {
		return nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(*photos), &paths); err != nil {
		return nil
	}
	return paths
}

// mergePhotoPaths merges existing and new photo paths, deduplicating.
func mergePhotoPaths(existing, newPaths []string) []string {
	seen := make(map[string]struct{}, len(existing))
	merged := make([]string, 0, len(existing)+len(newPaths))
	for _, p := range existing {
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	for _, p := range newPaths {
		if _, ok := seen[p]; !ok {
			merged = append(merged, p)
			seen[p] = struct{}{}
		}
	}
	return merged
}
