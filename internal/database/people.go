// Package database — people queries.
package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/carlj/godownload/internal/models"
)

// personColumns is the full column list for SELECT queries against people.
const personColumns = `id, name, aliases, birth_date, nationality,
	ethnicity, hair_color, eye_color, height, weight, measurements,
	tattoos, piercings, biography, photos, created_at`

// PeopleFilter holds optional filter parameters for ListPeople.
type PeopleFilter struct {
	Search *string // name LIKE
	Limit  int
	Offset int
}

// ListPeople returns a paginated list of people.
func (db *DB) ListPeople(ctx context.Context, f PeopleFilter) ([]models.Person, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := `SELECT ` + personColumns + ` FROM people WHERE 1=1`
	args := []any{}

	if f.Search != nil {
		query += " AND name LIKE ?"
		args = append(args, "%"+*f.Search+"%")
	}

	query += " ORDER BY name ASC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	people := []models.Person{}
	if err := db.SelectContext(ctx, &people, query, args...); err != nil {
		return nil, fmt.Errorf("listing people: %w", err)
	}
	return people, nil
}

// GetPerson retrieves a single person by ID.
func (db *DB) GetPerson(ctx context.Context, id int64) (*models.Person, error) {
	var p models.Person
	err := db.GetContext(ctx, &p,
		`SELECT `+personColumns+` FROM people WHERE id = ?`, id,
	)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting person %d: %w", id, err)
	}
	return &p, nil
}

// CreatePerson inserts a new person and populates ID and CreatedAt.
func (db *DB) CreatePerson(ctx context.Context, p *models.Person) error {
	result, err := db.ExecContext(ctx,
		`INSERT INTO people (name, aliases, birth_date, nationality,
		    ethnicity, hair_color, eye_color, height, weight, measurements,
		    tattoos, piercings, biography, photos)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.Aliases, p.BirthDate, p.Nationality,
		p.Ethnicity, p.HairColor, p.EyeColor, p.Height, p.Weight, p.Measurements,
		p.Tattoos, p.Piercings, p.Biography, p.Photos,
	)
	if err != nil {
		return fmt.Errorf("creating person: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting person id: %w", err)
	}
	p.ID = id
	p.CreatedAt = time.Now().UTC()
	return nil
}

// UpdatePerson updates mutable fields on an existing person.
func (db *DB) UpdatePerson(ctx context.Context, p *models.Person) error {
	_, err := db.ExecContext(ctx,
		`UPDATE people SET
		    name = ?, aliases = ?, birth_date = ?, nationality = ?,
		    ethnicity = ?, hair_color = ?, eye_color = ?,
		    height = ?, weight = ?, measurements = ?,
		    tattoos = ?, piercings = ?, biography = ?, photos = ?
		  WHERE id = ?`,
		p.Name, p.Aliases, p.BirthDate, p.Nationality,
		p.Ethnicity, p.HairColor, p.EyeColor,
		p.Height, p.Weight, p.Measurements,
		p.Tattoos, p.Piercings, p.Biography, p.Photos,
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("updating person %d: %w", p.ID, err)
	}
	return nil
}

// DeletePerson removes a person by ID.
func (db *DB) DeletePerson(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM people WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting person %d: %w", id, err)
	}
	return nil
}

// LinkGallery creates a gallery_persons association.
func (db *DB) LinkGallery(ctx context.Context, personID, galleryID int64) error {
	// Remove from unlinked blacklist if it exists (manual link overrides blacklist)
	_, _ = db.ExecContext(ctx, `DELETE FROM unlinked_gallery_persons WHERE gallery_id = ? AND person_id = ?`, galleryID, personID)

	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO gallery_persons (gallery_id, person_id) VALUES (?, ?)`,
		galleryID, personID,
	)
	if err != nil {
		return fmt.Errorf("linking gallery %d to person %d: %w", galleryID, personID, err)
	}
	return nil
}

// UnlinkGallery removes a gallery_persons association and records it in the blacklist.
func (db *DB) UnlinkGallery(ctx context.Context, personID, galleryID int64) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM gallery_persons WHERE gallery_id = ? AND person_id = ?`,
		galleryID, personID,
	)
	if err != nil {
		return fmt.Errorf("unlinking gallery %d from person %d: %w", galleryID, personID, err)
	}

	// Record unlinking to prevent auto-relinking
	_, _ = db.ExecContext(ctx,
		`INSERT OR IGNORE INTO unlinked_gallery_persons (gallery_id, person_id) VALUES (?, ?)`,
		galleryID, personID,
	)

	return nil
}

// ListPersonIdentifiers returns all external identifiers for a person.
func (db *DB) ListPersonIdentifiers(ctx context.Context, personID int64) ([]models.PersonIdentifier, error) {
	ids := []models.PersonIdentifier{}
	err := db.SelectContext(ctx, &ids,
		`SELECT id, person_id, provider, external_id, created_at
		   FROM person_identifiers
		  WHERE person_id = ?
		  ORDER BY provider ASC`, personID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing identifiers for person %d: %w", personID, err)
	}
	return ids, nil
}

// UpsertPersonIdentifier inserts or replaces an external identifier for a person.
func (db *DB) UpsertPersonIdentifier(ctx context.Context, pid *models.PersonIdentifier) error {
	result, err := db.ExecContext(ctx,
		`INSERT INTO person_identifiers (person_id, provider, external_id)
		 VALUES (?, ?, ?)
		 ON CONFLICT (provider, external_id) DO UPDATE SET person_id = excluded.person_id`,
		pid.PersonID, pid.Provider, pid.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("upserting person identifier: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting person identifier id: %w", err)
	}
	pid.ID = id
	pid.CreatedAt = time.Now().UTC()
	return nil
}

// ListPersonGalleries returns all galleries linked to a person.
func (db *DB) ListPersonGalleries(ctx context.Context, personID int64, limit, offset int) ([]models.Gallery, error) {
	if limit <= 0 {
		limit = 50
	}
	galleries := []models.Gallery{}
	err := db.SelectContext(ctx, &galleries,
		`SELECT g.id, g.source_id, g.provider, g.provider_gallery_id, g.title,
		        g.url, g.thumbnail_url, g.local_thumbnail_path, g.description,
		        g.rating, g.release_date, g.source_url, g.provider_thumbnail_url,
		        g.created_at
		   FROM galleries g
		   JOIN gallery_persons gp ON gp.gallery_id = g.id
		  WHERE gp.person_id = ?
		  ORDER BY g.created_at DESC
		  LIMIT ? OFFSET ?`, personID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing galleries for person %d: %w", personID, err)
	}
	return galleries, nil
}

// CountPeople returns the total number of people.
func (db *DB) CountPeople(ctx context.Context) (int64, error) {
	var count int64
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM people`); err != nil {
		return 0, fmt.Errorf("counting people: %w", err)
	}
	return count, nil
}

// FindGalleriesByTitleMatch returns gallery IDs whose title contains the
// given name (case-insensitive). Used for auto-linking.
func (db *DB) FindGalleriesByTitleMatch(ctx context.Context, name string) ([]int64, error) {
	rows := []struct {
		ID int64 `db:"id"`
	}{}
	err := db.SelectContext(ctx, &rows,
		`SELECT id FROM galleries WHERE title LIKE ? COLLATE NOCASE`,
		"%"+name+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("finding galleries matching title %q: %w", name, err)
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}

// FindGalleriesBySourceURLMatch returns gallery IDs whose source_url contains the
// given pattern (case-insensitive). Used for auto-linking.
func (db *DB) FindGalleriesBySourceURLMatch(ctx context.Context, pattern string) ([]int64, error) {
	rows := []struct {
		ID int64 `db:"id"`
	}{}
	err := db.SelectContext(ctx, &rows,
		`SELECT id FROM galleries WHERE source_url LIKE ? COLLATE NOCASE`,
		"%"+pattern+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("finding galleries matching source_url %q: %w", pattern, err)
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}

// IsGalleryUnlinked checks if a person was specifically unlinked from a gallery.
func (db *DB) IsGalleryUnlinked(ctx context.Context, personID, galleryID int64) (bool, error) {
	var count int
	err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM unlinked_gallery_persons WHERE gallery_id = ? AND person_id = ?`, galleryID, personID)
	if err != nil {
		return false, fmt.Errorf("checking unlinked status: %w", err)
	}
	return count > 0, nil
}

// MergePeople merges one or more people into a single "keep" person.
// Gallery links and identifiers from the merged people are reassigned to the keeper.
// Aliases from the merged people are appended. The merged people are then deleted.
// All operations run within a single transaction.
func (db *DB) MergePeople(ctx context.Context, keepID int64, mergeIDs []int64) error {
	if len(mergeIDs) == 0 {
		return nil
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning merge transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Load the keeper.
	var keeper models.Person
	if err := tx.GetContext(ctx, &keeper,
		`SELECT `+personColumns+` FROM people WHERE id = ?`, keepID,
	); err != nil {
		return fmt.Errorf("loading keeper person %d: %w", keepID, err)
	}

	// Build placeholders for mergeIDs.
	placeholders := make([]string, len(mergeIDs))
	mergeArgs := make([]any, len(mergeIDs))
	for i, mid := range mergeIDs {
		placeholders[i] = "?"
		mergeArgs[i] = mid
	}
	inClause := strings.Join(placeholders, ",")

	// Collect aliases from merged people.
	var mergedNames []string
	rows, err := tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT name, aliases FROM people WHERE id IN (%s)`, inClause),
		mergeArgs...,
	)
	if err != nil {
		return fmt.Errorf("loading merged people: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var aliases *string
		if err := rows.Scan(&name, &aliases); err != nil {
			return fmt.Errorf("scanning merged person: %w", err)
		}
		mergedNames = append(mergedNames, name)
		if aliases != nil && *aliases != "" {
			mergedNames = append(mergedNames, strings.Split(*aliases, ", ")...)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating merged people: %w", err)
	}

	// Append merged aliases to keeper.
	if len(mergedNames) > 0 {
		existing := ""
		if keeper.Aliases != nil {
			existing = *keeper.Aliases
		}
		parts := []string{}
		if existing != "" {
			parts = append(parts, existing)
		}
		parts = append(parts, mergedNames...)
		combined := strings.Join(parts, ", ")
		if _, err := tx.ExecContext(ctx,
			`UPDATE people SET aliases = ? WHERE id = ?`, combined, keepID,
		); err != nil {
			return fmt.Errorf("updating keeper aliases: %w", err)
		}
	}

	// Reassign gallery links. Use INSERT OR IGNORE to handle duplicates.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`INSERT OR IGNORE INTO gallery_persons (gallery_id, person_id)
		             SELECT gallery_id, ? FROM gallery_persons WHERE person_id IN (%s)`, inClause),
		append([]any{keepID}, mergeArgs...)...,
	); err != nil {
		return fmt.Errorf("reassigning gallery links: %w", err)
	}

	// Reassign identifiers. Use INSERT OR IGNORE to handle duplicates.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE OR IGNORE person_identifiers SET person_id = ? WHERE person_id IN (%s)`, inClause),
		append([]any{keepID}, mergeArgs...)...,
	); err != nil {
		return fmt.Errorf("reassigning identifiers: %w", err)
	}

	// Delete old gallery links for merged people.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM gallery_persons WHERE person_id IN (%s)`, inClause),
		mergeArgs...,
	); err != nil {
		return fmt.Errorf("cleaning gallery links: %w", err)
	}

	// Delete old identifiers for merged people.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM person_identifiers WHERE person_id IN (%s)`, inClause),
		mergeArgs...,
	); err != nil {
		return fmt.Errorf("cleaning identifiers: %w", err)
	}

	// Delete merged people.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM people WHERE id IN (%s)`, inClause),
		mergeArgs...,
	); err != nil {
		return fmt.Errorf("deleting merged people: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing merge transaction: %w", err)
	}

	return nil
}

// BulkDeletePeople deletes multiple people by IDs and returns the number deleted.
func (db *DB) BulkDeletePeople(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning bulk delete transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	// Clean up related records.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM gallery_persons WHERE person_id IN (%s)`, inClause),
		args...,
	); err != nil {
		return 0, fmt.Errorf("deleting gallery links: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM person_identifiers WHERE person_id IN (%s)`, inClause),
		args...,
	); err != nil {
		return 0, fmt.Errorf("deleting identifiers: %w", err)
	}

	result, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM people WHERE id IN (%s)`, inClause),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("deleting people: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing bulk delete: %w", err)
	}

	return n, nil
}
