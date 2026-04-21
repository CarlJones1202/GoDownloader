-- 004_unlinked_blacklist.sql
-- Table to store manual unlinks to prevent auto-linker from re-linking them.

CREATE TABLE IF NOT EXISTS unlinked_gallery_persons (
    gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    person_id  INTEGER NOT NULL REFERENCES people(id)   ON DELETE CASCADE,
    PRIMARY KEY (gallery_id, person_id)
);

CREATE INDEX IF NOT EXISTS idx_unlinked_gallery_persons_gallery ON unlinked_gallery_persons(gallery_id);
CREATE INDEX IF NOT EXISTS idx_unlinked_gallery_persons_person  ON unlinked_gallery_persons(person_id);
