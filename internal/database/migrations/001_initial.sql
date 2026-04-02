-- 001_initial.sql
-- Initial schema for GoDownload

CREATE TABLE IF NOT EXISTS sources (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    url             TEXT    NOT NULL UNIQUE,
    name            TEXT    NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    priority        INTEGER NOT NULL DEFAULT 0,
    last_crawled_at DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS galleries (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id              INTEGER REFERENCES sources(id) ON DELETE SET NULL,
    provider               TEXT,
    provider_gallery_id    TEXT,
    title                  TEXT,
    url                    TEXT,
    thumbnail_url          TEXT,
    local_thumbnail_path   TEXT,
    description            TEXT,
    rating                 REAL,
    release_date           TEXT,
    source_url             TEXT,
    provider_thumbnail_url TEXT,
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS images (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    gallery_id       INTEGER REFERENCES galleries(id) ON DELETE CASCADE,
    filename         TEXT    NOT NULL,
    original_url     TEXT,
    width            INTEGER,
    height           INTEGER,
    duration_seconds INTEGER,
    file_hash        TEXT,
    dominant_colors  TEXT,
    is_video         BOOLEAN NOT NULL DEFAULT false,
    vr_mode          TEXT    NOT NULL DEFAULT 'none',
    is_favorite      BOOLEAN NOT NULL DEFAULT false,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS people (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    aliases     TEXT,
    birth_date  DATE,
    nationality TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS person_identifiers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id   INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    provider    TEXT    NOT NULL,
    external_id TEXT    NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider, external_id)
);

CREATE TABLE IF NOT EXISTS gallery_persons (
    gallery_id INTEGER NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    person_id  INTEGER NOT NULL REFERENCES people(id)   ON DELETE CASCADE,
    PRIMARY KEY (gallery_id, person_id)
);

CREATE TABLE IF NOT EXISTS tags (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT NOT NULL UNIQUE,
    category TEXT
);

CREATE TABLE IF NOT EXISTS image_tags (
    image_id   INTEGER NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    tag_id     INTEGER NOT NULL REFERENCES tags(id)   ON DELETE CASCADE,
    confidence REAL,
    PRIMARY KEY (image_id, tag_id)
);

CREATE TABLE IF NOT EXISTS download_queue (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    type          TEXT    NOT NULL,
    url           TEXT    NOT NULL,
    target_id     INTEGER,
    status        TEXT    NOT NULL DEFAULT 'pending',
    retry_count   INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_galleries_source         ON galleries(source_id);
CREATE INDEX IF NOT EXISTS idx_galleries_provider       ON galleries(provider, provider_gallery_id);
CREATE INDEX IF NOT EXISTS idx_images_gallery           ON images(gallery_id);
CREATE INDEX IF NOT EXISTS idx_images_hash              ON images(file_hash);
CREATE INDEX IF NOT EXISTS idx_images_favorite          ON images(is_favorite);
CREATE INDEX IF NOT EXISTS idx_people_name              ON people(name);
CREATE INDEX IF NOT EXISTS idx_person_identifiers       ON person_identifiers(provider, external_id);
CREATE INDEX IF NOT EXISTS idx_gallery_persons_gallery  ON gallery_persons(gallery_id);
CREATE INDEX IF NOT EXISTS idx_gallery_persons_person   ON gallery_persons(person_id);
CREATE INDEX IF NOT EXISTS idx_download_queue_status    ON download_queue(status);
CREATE INDEX IF NOT EXISTS idx_download_queue_type      ON download_queue(type);
