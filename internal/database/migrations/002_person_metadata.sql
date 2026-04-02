-- 002_person_metadata.sql
-- Extend people table with full metadata fields from providers
-- (ethnicity, physical attributes, bio, photos, etc.)

ALTER TABLE people ADD COLUMN ethnicity    TEXT;
ALTER TABLE people ADD COLUMN hair_color   TEXT;
ALTER TABLE people ADD COLUMN eye_color    TEXT;
ALTER TABLE people ADD COLUMN height       TEXT;  -- e.g. "170cm"
ALTER TABLE people ADD COLUMN weight       TEXT;  -- e.g. "55kg"
ALTER TABLE people ADD COLUMN measurements TEXT;  -- e.g. "34B-24-34"
ALTER TABLE people ADD COLUMN tattoos      TEXT;
ALTER TABLE people ADD COLUMN piercings    TEXT;
ALTER TABLE people ADD COLUMN biography    TEXT;
ALTER TABLE people ADD COLUMN photos       TEXT;  -- JSON array of local photo paths
