-- 003_person_photo_urls.sql
-- Store original photo URLs for people to enable re-downloading when files are missing

CREATE TABLE IF NOT EXISTS person_photo_urls (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id  INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    photo_url  TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_person_photo_urls_person ON person_photo_urls(person_id);
