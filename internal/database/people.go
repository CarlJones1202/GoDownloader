// Package database — people queries.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/carlj/godownload/internal/models"
)

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

	query := `SELECT id, name, aliases, birth_date, nationality, created_at
	            FROM people WHERE 1=1`
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
		`SELECT id, name, aliases, birth_date, nationality, created_at
		   FROM people WHERE id = ?`, id,
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
		`INSERT INTO people (name, aliases, birth_date, nationality) VALUES (?, ?, ?, ?)`,
		p.Name, p.Aliases, p.BirthDate, p.Nationality,
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
		`UPDATE people SET name = ?, aliases = ?, birth_date = ?, nationality = ?
		  WHERE id = ?`,
		p.Name, p.Aliases, p.BirthDate, p.Nationality, p.ID,
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
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO gallery_persons (gallery_id, person_id) VALUES (?, ?)`,
		galleryID, personID,
	)
	if err != nil {
		return fmt.Errorf("linking gallery %d to person %d: %w", galleryID, personID, err)
	}
	return nil
}

// UnlinkGallery removes a gallery_persons association.
func (db *DB) UnlinkGallery(ctx context.Context, personID, galleryID int64) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM gallery_persons WHERE gallery_id = ? AND person_id = ?`,
		galleryID, personID,
	)
	if err != nil {
		return fmt.Errorf("unlinking gallery %d from person %d: %w", galleryID, personID, err)
	}
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
		        g.url, g.thumbnail_url, g.local_thumbnail_path, g.created_at
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
		return nil, fmt.Errorf("finding galleries matching %q: %w", name, err)
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}
