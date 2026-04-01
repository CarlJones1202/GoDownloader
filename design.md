# AG-Godownload - Design Document

## 1. Project Overview

A Go-based image gallery API with automatic crawling, image ripping, and performer metadata management. The system downloads adult content galleries from source providers, extracts metadata from metadata providers, downloads images from image providers, and organizes everything with people profiles.

---

## 2. Provider Taxonomy

| Type | Description | Examples |
|------|-------------|----------|
| **Source Providers** | Where galleries can be crawled/downloaded | ViperGirls, JKForum, Kitty-Kats, image boards |
| **Metadata Providers** | Where people/gallery info is extracted from | StashDB (people), FreeOnes (people), MetArt (galleries+people), Playboy, Babepedia |
| **Image Providers** | Where individual images are hosted | Imx.to, TurboImageHost, ImageBam, PixHost, Vipr.im |

*Note: Some providers are hybrid (MetArt provides both metadata and hosts images)*

---

## 3. Core Features (MVP)

### Gallery Management
- CRUD operations for galleries
- Source provider association
- Metadata provider association
- Manual override/entry
- Gallery linking to people

### Image Management
- Download from image providers
- Thumbnail generation
- Color extraction (K-means)
- Metadata storage (dimensions, duration)
- Favorite/unfavorite

### Video Management
- Download from video providers (YouTube, Pornhub, etc.)
- Trickplay data generation (sprite sheets + VTT)
- VR mode flag (180/360)

### People Management
- Person profiles with metadata
- Provider aliases
- External identifiers (StashDB ID, FreeOnes slug, etc.)
- Auto-linking to galleries

### Source Crawling
- Configurable crawl intervals
- Priority queue
- Concurrent downloading
- Re-crawl functionality

### Queue Management
- View pending downloads
- Pause/resume queue
- Retry failed items
- Clear queue

---

## 4. External Integrations

### Metadata Providers

| Provider | Type | Status | Data Provided |
|----------|------|--------|---------------|
| StashDB | GraphQL API | Working | People (full profile, measurements, tattoos, social) |
| FreeOnes | HTML Scraper | Working | People (basic profile) |
| Babepedia | HTML Scraper | Working | People (basic profile) |
| MetArt | API | Working | Galleries + People |
| MetartX | API | Working | Galleries + People |
| Playboy | API | Working | Galleries + People |
| Vixen/SexArt/LifeErotic | API | Working | Galleries |

### Image Providers (Rippers)

ImageBam, ImgBox, Imx.to, TurboImageHost, Vipr.im, PixHost, PostImages, Imagetwist, AcidImg, MyMyPic

### Video Providers

YouTube, Pornhub, TnaFlix, PMVHaven (via yt-dlp)

---

## 5. SQL Efficiency Requirements

This is a **massive DB** - high efficiency SQL is critical.

### Indexing Strategy
- All foreign key columns indexed
- Composite indexes for common query patterns
- Index on `people.name`, `galleries.title`, `images.file_hash`
- Partial indexes for filtered queries
- Index on `download_queue.status` for queue queries

### Query Patterns to Optimize
- Gallery → Images (paginated)
- Person → Galleries (paginated)
- Source → Pending downloads
- Color-based image search (range queries)
- Tag-based filtering

### Monitoring & Enforcement
- Log slow queries (>100ms) in development
- Require `EXPLAIN QUERY PLAN` for complex queries
- Use batch inserts (`INSERT INTO ... VALUES (...), (...), (...)`) for bulk operations
- Use transactions for related operations

---

## 6. Admin Tools

| Tool | Priority | Description |
|------|----------|-------------|
| Statistics Dashboard | High | Gallery count, image count, download stats, provider breakdown |
| Re-crawl/Re-download | High | Trigger full re-crawl or re-download for sources/images/videos |
| Queue Management | High | View pending, failed, retry, clear queue |
| Gallery Cleanup | High | Find/remove orphaned images not part of any gallery |
| Person Management | Medium | Fix incorrect metadata, bulk operations |
| Gallery Linking | Medium | Manually link/unlink galleries to people |

---

## 7. Broken / Incomplete Items (Keep in Mind)

### TODO (Not Yet Implemented)
- [ ] AI Image Matching (perceptual similarity via CLIP embeddings)
- [ ] Search by image similarity
- [ ] Search by color (extraction done, UI search not)
- [ ] AI auto-extract name from source URL
- [ ] Favoriting system (needs database + UI)
- [ ] Auto-link galleries on person/alias creation (partial)
- [ ] VR video playback (flag exists, player incomplete)

### Known Bugs/Issues
- Videos load entirely into memory before writing (memory issue for large files)
- Thumbnail generation incomplete
- Inconsistent HTTP clients across services
- No graceful shutdown
- Slow SQL warnings not being resolved

---

## 8. Tech Stack

| Layer | Technology | Notes |
|-------|------------|-------|
| Backend | Go + Gin | Current - stays familiar |
| Database | SQLite | Per requirement |
| Frontend | React + TypeScript | Type safety for maintainability |
| State/Fetch | React Query (TanStack) | Handles caching, pagination |
| Styling | Tailwind CSS | Rapid development |
| HTTP Client | Custom with connection pooling | Refactor existing implementation |

---

## 9. Proposed File Structure

```
/cmd/
  /api/                    # Main API server entrypoint
  /migrate/                # Database migration tool
/internal/
  /config/                 # Configuration management
  /database/               # DB connection, queries, migrations
  /handlers/               # HTTP request handlers
  /middleware/             # Auth, rate limiting, logging
  /models/                 # Data models
  /services/               # Business logic
    /crawler/              # Source crawling
    /ripper/               # Image/video rippers
    /providers/            # External provider clients
    /queue/                # Download queue management
    /workers/              # Background workers
  /utils/                  # Shared utilities
/web/                      # Frontend React app
/migrations/               # SQL migration files
```

### Key Principles
- **Clear separation** - Each provider in its own file/package
- **Admin-friendly** - Routes for all admin operations
- **Testable** - Dependency injection for services
- **Fast queries** - Queries co-located with models, proper indexing

---

## 10. Database Schema

### Sources Table
```sql
CREATE TABLE sources (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 0,
    last_crawled_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Galleries Table
```sql
CREATE TABLE galleries (
    id INTEGER PRIMARY KEY,
    source_id INTEGER REFERENCES sources(id),
    provider TEXT,
    provider_gallery_id TEXT,
    title TEXT,
    url TEXT,
    thumbnail_url TEXT,
    local_thumbnail_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Images Table
```sql
CREATE TABLE images (
    id INTEGER PRIMARY KEY,
    gallery_id INTEGER REFERENCES galleries(id),
    filename TEXT NOT NULL,
    original_url TEXT,
    width INTEGER,
    height INTEGER,
    duration_seconds INTEGER,
    file_hash TEXT,
    dominant_colors TEXT,
    is_video BOOLEAN DEFAULT false,
    vr_mode TEXT DEFAULT 'none',
    is_favorite BOOLEAN DEFAULT false,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### People Table
```sql
CREATE TABLE people (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    aliases TEXT,
    birth_date DATE,
    nationality TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Person Identifiers Table
```sql
CREATE TABLE person_identifiers (
    id INTEGER PRIMARY KEY,
    person_id INTEGER REFERENCES people(id),
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Gallery-Person Links Table
```sql
CREATE TABLE gallery_persons (
    gallery_id INTEGER REFERENCES galleries(id),
    person_id INTEGER REFERENCES people(id),
    PRIMARY KEY (gallery_id, person_id)
);
```

### Tags Table
```sql
CREATE TABLE tags (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    category TEXT
);
```

### Image Tags Table
```sql
CREATE TABLE image_tags (
    image_id INTEGER REFERENCES images(id),
    tag_id INTEGER REFERENCES tags(id),
    confidence REAL,
    PRIMARY KEY (image_id, tag_id)
);
```

### Download Queue Table
```sql
CREATE TABLE download_queue (
    id INTEGER PRIMARY KEY,
    type TEXT NOT NULL,
    url TEXT NOT NULL,
    target_id INTEGER,
    status TEXT DEFAULT 'pending',
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Required Indexes
```sql
CREATE INDEX idx_galleries_source ON galleries(source_id);
CREATE INDEX idx_galleries_provider ON galleries(provider, provider_gallery_id);
CREATE INDEX idx_images_gallery ON images(gallery_id);
CREATE INDEX idx_images_hash ON images(file_hash);
CREATE INDEX idx_images_favorite ON images(is_favorite);
CREATE INDEX idx_people_name ON people(name);
CREATE INDEX idx_person_identifiers ON person_identifiers(provider, external_id);
CREATE INDEX idx_gallery_persons_gallery ON gallery_persons(gallery_id);
CREATE INDEX idx_gallery_persons_person ON gallery_persons(person_id);
CREATE INDEX idx_download_queue_status ON download_queue(status);
CREATE INDEX idx_download_queue_type ON download_queue(type);
```

---

## 11. API Endpoints Overview

### Sources
- `GET /sources` - List all sources
- `POST /sources` - Create source
- `POST /sources/:id/crawl` - Trigger crawl
- `POST /sources/:id/recrawl` - Re-crawl all galleries
- `DELETE /sources/:id` - Delete source

### Galleries
- `GET /galleries` - List galleries (paginated, filterable)
- `GET /galleries/:id` - Get gallery details
- `POST /galleries` - Create gallery
- `PUT /galleries/:id` - Update gallery
- `DELETE /galleries/:id` - Delete gallery
- `POST /galleries/:id/images` - Add image to gallery

### Images
- `GET /images` - List images (paginated, filterable)
- `GET /images/:id` - Get image details
- `DELETE /images/:id` - Delete image
- `POST /images/:id/favorite` - Toggle favorite
- `GET /images/search/color` - Search by color

### Videos
- `GET /videos` - List videos
- `POST /videos/:id/redownload` - Re-download video

### People
- `GET /people` - List people
- `GET /people/:id` - Get person details
- `POST /people` - Create person
- `PUT /people/:id` - Update person
- `DELETE /people/:id` - Delete person
- `POST /people/:id/link-gallery/:galleryId` - Link gallery
- `POST /people/:id/unlink-gallery/:galleryId` - Unlink gallery

### Admin
- `GET /admin/stats` - Statistics dashboard
- `GET /admin/queue` - View download queue
- `POST /admin/queue/:id/retry` - Retry failed download
- `DELETE /admin/queue/:id` - Remove from queue
- `POST /admin/galleries/cleanup` - Find orphaned images
- `POST /admin/sources/:id/recrawl` - Re-crawl source

---

## 12. Migration Strategy

1. Create new database schema using migrations
2. Write migration tool to copy data from old DB
3. Validate data integrity
4. Swap databases
5. Run verification tasks

*Note: Migration will be handled separately*

---

## 13. Priorities for Rewrite

### Phase 1: Foundation
1. Clean project structure
2. Database schema with proper indexes
3. Core CRUD handlers
4. Source crawling

### Phase 2: Core Features
5. Image/video downloading
6. Queue management
7. People management

### Phase 3: External Integrations
8. Metadata providers (StashDB, FreeOnes, etc.)
9. Image providers (rippers)

### Phase 4: Admin Tools
10. Statistics dashboard
11. Re-crawl/re-download
12. Gallery cleanup

### Phase 5: Polish
13. Favoriting
14. Color search
15. Auto-linking