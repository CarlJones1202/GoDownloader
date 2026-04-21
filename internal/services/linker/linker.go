// Package linker provides automatic gallery-to-person linking.
// When a person is created or aliases are updated, the linker searches
// gallery titles and source URLs for name matches and creates gallery_persons links.
package linker

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/models"
)

// AutoLinker automatically links people to galleries based on name/alias matches.
type AutoLinker struct {
	db *database.DB
}

// New creates an AutoLinker.
func New(db *database.DB) *AutoLinker {
	return &AutoLinker{db: db}
}

// LinkPerson searches gallery titles and source URLs for the person's name and aliases,
// and creates gallery_persons records for any matches.
// It returns the number of new links created.
func (al *AutoLinker) LinkPerson(ctx context.Context, person *models.Person) (int, error) {
	searchTerms := al.collectSearchTerms(person)
	if len(searchTerms) == 0 {
		return 0, nil
	}

	linked := 0
	seenGalleries := make(map[int64]bool)

	for _, term := range searchTerms {
		slog.Debug("autolink: searching titles", "term", term)
		titleGIDs, err := al.db.FindGalleriesByTitleMatch(ctx, term)
		if err != nil {
			slog.Warn("autolink: error searching titles", "term", term, "error", err)
		}

		// Search in SourceURL
		slog.Debug("autolink: searching urls", "term", term)
		urlGIDs, err := al.db.FindGalleriesBySourceURLMatch(ctx, term)
		if err != nil {
			slog.Warn("autolink: error searching source urls", "term", term, "error", err)
		}

		allGIDs := append(titleGIDs, urlGIDs...)

		for _, gid := range allGIDs {
			if seenGalleries[gid] {
				continue
			}
			seenGalleries[gid] = true

			// Skip if manually unlinked
			unlinked, err := al.db.IsGalleryUnlinked(ctx, person.ID, gid)
			if err != nil {
				slog.Warn("autolink: error checking unlinked status", "person_id", person.ID, "gallery_id", gid, "error", err)
				continue
			}
			if unlinked {
				continue
			}

			if err := al.db.LinkGallery(ctx, person.ID, gid); err != nil {
				slog.Warn("autolink: error linking gallery",
					"person_id", person.ID,
					"gallery_id", gid,
					"error", err,
				)
				continue
			}
			linked++
		}
	}

	if linked > 0 {
		slog.Info("autolink: linked galleries",
			"person_id", person.ID,
			"person_name", person.Name,
			"links_created", linked,
		)
	}

	return linked, nil
}

// ScanAllGalleries iterates through all people and attempts to link them to galleries.
func (al *AutoLinker) ScanAllGalleries(ctx context.Context) (int, error) {
	people, err := al.db.ListPeople(ctx, database.PeopleFilter{Limit: -1})
	if err != nil {
		return 0, err
	}

	totalLinked := 0
	for _, p := range people {
		linked, err := al.LinkPerson(ctx, &p)
		if err != nil {
			slog.Error("autolink: scan failed for person", "person_id", p.ID, "error", err)
			continue
		}
		totalLinked += linked
	}

	return totalLinked, nil
}

// LinkGallery finds all people whose name or aliases match the gallery title
// or source URL and creates links for them.
func (al *AutoLinker) LinkGallery(ctx context.Context, gallery *models.Gallery) (int, error) {
	if gallery == nil {
		return 0, nil
	}

	people, err := al.db.ListPeople(ctx, database.PeopleFilter{Limit: -1})
	if err != nil {
		return 0, err
	}

	linked := 0
	for _, p := range people {
		searchTerms := al.collectSearchTerms(&p)

		match := false
		for _, term := range searchTerms {
			// Check Title
			if gallery.Title != nil && strings.Contains(strings.ToLower(*gallery.Title), strings.ToLower(term)) {
				slog.Debug("autolink: match found in title", "person", p.Name, "gallery", *gallery.Title, "term", term)
				match = true
				break
			}
			// Check SourceURL
			if gallery.SourceURL != nil && strings.Contains(strings.ToLower(*gallery.SourceURL), strings.ToLower(term)) {
				slog.Debug("autolink: match found in source_url", "person", p.Name, "url", *gallery.SourceURL, "term", term)
				match = true
				break
			}
			// Backwards compatibility/fallback to 'URL' if SourceURL is nil
			if gallery.SourceURL == nil && gallery.URL != nil && strings.Contains(strings.ToLower(*gallery.URL), strings.ToLower(term)) {
				slog.Debug("autolink: match found in url", "person", p.Name, "url", *gallery.URL, "term", term)
				match = true
				break
			}
		}

		if match {
			// Skip if manually unlinked
			unlinked, err := al.db.IsGalleryUnlinked(ctx, p.ID, gallery.ID)
			if err != nil {
				slog.Warn("autolink: error checking unlinked status", "person_id", p.ID, "gallery_id", gallery.ID, "error", err)
				continue
			}
			if unlinked {
				continue
			}

			if err := al.db.LinkGallery(ctx, p.ID, gallery.ID); err != nil {
				slog.Warn("autolink: error linking gallery",
					"person_id", p.ID,
					"gallery_id", gallery.ID,
					"error", err,
				)
				continue
			}
			linked++
		}
	}

	if linked > 0 {
		slog.Info("autolink: linked gallery to people",
			"gallery_id", gallery.ID,
			"links_created", linked,
		)
	}

	return linked, nil
}

// collectSearchTerms returns a list of names/aliases and their common variations
// and URL-encoded forms for searching.
func (al *AutoLinker) collectSearchTerms(person *models.Person) []string {
	names := []string{person.Name}

	if person.Aliases != nil && *person.Aliases != "" {
		for _, alias := range strings.Split(*person.Aliases, ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				names = append(names, alias)
			}
		}
	}

	uniqueTerms := make(map[string]bool)
	for _, n := range names {
		// Base name
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		uniqueTerms[n] = true

		// "First Last" <-> "Last, First" variations
		if strings.Contains(n, " ") && !strings.Contains(n, ",") {
			// Assume "First Last" -> generate "Last, First"
			parts := strings.Fields(n)
			if len(parts) >= 2 {
				last := parts[len(parts)-1]
				firsts := strings.Join(parts[:len(parts)-1], " ")
				variant := last + ", " + firsts
				uniqueTerms[variant] = true
			}
		} else if strings.Contains(n, ",") {
			// Assume "Last, First" -> generate "First Last"
			parts := strings.SplitN(n, ",", 2)
			if len(parts) == 2 {
				last := strings.TrimSpace(parts[0])
				first := strings.TrimSpace(parts[1])
				variant := first + " " + last
				uniqueTerms[variant] = true
			}
		}
	}

	// Add URL-encoded versions
	finalTerms := make([]string, 0, len(uniqueTerms)*2)
	for term := range uniqueTerms {
		finalTerms = append(finalTerms, term)

		// URL encoded (space -> %20, comma -> %2C)
		encoded := url.QueryEscape(term)
		if encoded != term {
			finalTerms = append(finalTerms, encoded)
		}

		// Some URLs use + for spaces instead of %20
		if strings.Contains(term, " ") {
			plus := strings.ReplaceAll(term, " ", "+")
			finalTerms = append(finalTerms, plus)

			hyphen := strings.ReplaceAll(term, " ", "-")
			finalTerms = append(finalTerms, hyphen)

			underscore := strings.ReplaceAll(term, " ", "_")
			finalTerms = append(finalTerms, underscore)
		}
	}

	// Final deduplication and lowercase normalization for consistent matching
	dedupMap := make(map[string]bool)
	result := make([]string, 0, len(finalTerms))
	for _, t := range finalTerms {
		low := strings.ToLower(t)
		if !dedupMap[low] {
			dedupMap[low] = true
			result = append(result, low)
		}
	}

	return result
}
