// Package handlers — people HTTP handlers.
package handlers

import (
	"net/http"
	"strings"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/providers"
	"github.com/gin-gonic/gin"
)

// PeopleHandler handles HTTP requests for the /api/v1/people resource.
type PeopleHandler struct {
	db       *database.DB
	linker   *linker.AutoLinker
	enricher *providers.Enricher
}

// NewPeopleHandler creates a PeopleHandler.
func NewPeopleHandler(db *database.DB, al *linker.AutoLinker, enricher *providers.Enricher) *PeopleHandler {
	return &PeopleHandler{db: db, linker: al, enricher: enricher}
}

// RegisterRoutes registers all people routes on the given group.
func (h *PeopleHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.GET("/:id", h.get)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
	rg.GET("/:id/galleries", h.listGalleries)
	rg.POST("/:id/link-gallery/:galleryId", h.linkGallery)
	rg.POST("/:id/unlink-gallery/:galleryId", h.unlinkGallery)
	rg.GET("/:id/identifiers", h.listIdentifiers)
	rg.POST("/:id/identifiers", h.upsertIdentifier)
	rg.POST("/:id/enrich", h.enrich)
}

type createPersonRequest struct {
	Name        string  `json:"name"        binding:"required"`
	Aliases     *string `json:"aliases"`
	BirthDate   *string `json:"birth_date"` // ISO 8601 date
	Nationality *string `json:"nationality"`
}

type updatePersonRequest struct {
	Name        *string `json:"name"`
	Aliases     *string `json:"aliases"`
	BirthDate   *string `json:"birth_date"`
	Nationality *string `json:"nationality"`
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
// the external ID as an identifier, and returns the updated person.
func (h *PeopleHandler) applyPersonInfo(c *gin.Context, p *models.Person, info *providers.PersonInfo, providerName string) {
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
