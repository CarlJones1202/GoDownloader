-- 002_unlinked_galleries.sql
-- Tracking manual unlinking of galleries from people to prevent auto-relinking.

CREATE TABLE IF NOT EXISTS unlinked_gallery_persons (
    gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    person_id  INTEGER NOT NULL REFERENCES people(id)   ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (gallery_id, person_id)
);

CREATE INDEX IF NOT EXISTS idx_unlinked_gallery_persons_person ON unlinked_gallery_persons(person_id);
