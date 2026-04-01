// Package linker provides automatic gallery-to-person linking.
// When a person is created or aliases are updated, the linker searches
// gallery titles for name matches and creates gallery_persons links.
package linker

import (
	"context"
	"log/slog"
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

// LinkPerson searches gallery titles for the person's name and aliases,
// and creates gallery_persons records for any matches.
// It returns the number of new links created.
func (al *AutoLinker) LinkPerson(ctx context.Context, person *models.Person) (int, error) {
	names := al.collectNames(person)
	if len(names) == 0 {
		return 0, nil
	}

	linked := 0
	seen := make(map[int64]bool)

	for _, name := range names {
		galleryIDs, err := al.db.FindGalleriesByTitleMatch(ctx, name)
		if err != nil {
			slog.Warn("autolink: searching galleries", "name", name, "error", err)
			continue
		}

		for _, gid := range galleryIDs {
			if seen[gid] {
				continue
			}
			seen[gid] = true

			if err := al.db.LinkGallery(ctx, person.ID, gid); err != nil {
				slog.Warn("autolink: linking gallery",
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

// collectNames returns the person's name plus all aliases as a slice.
func (al *AutoLinker) collectNames(person *models.Person) []string {
	names := []string{person.Name}

	if person.Aliases != nil && *person.Aliases != "" {
		for _, alias := range strings.Split(*person.Aliases, ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				names = append(names, alias)
			}
		}
	}

	return names
}
