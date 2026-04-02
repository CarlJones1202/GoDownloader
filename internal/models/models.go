// Package models defines the domain data structures that map to database tables.
package models

import "time"

// Source represents a crawlable content source (e.g. a forum thread, board, or gallery site).
type Source struct {
	ID            int64      `db:"id"             json:"id"`
	URL           string     `db:"url"            json:"url"`
	Name          string     `db:"name"           json:"name"`
	Enabled       bool       `db:"enabled"        json:"enabled"`
	Priority      int        `db:"priority"       json:"priority"`
	LastCrawledAt *time.Time `db:"last_crawled_at" json:"last_crawled_at,omitempty"`
	CreatedAt     time.Time  `db:"created_at"     json:"created_at"`
}

// Gallery represents a collection of images from a single source post or provider page.
type Gallery struct {
	ID                   int64     `db:"id"                      json:"id"`
	SourceID             *int64    `db:"source_id"               json:"source_id,omitempty"`
	Provider             *string   `db:"provider"                json:"provider,omitempty"`
	ProviderGalleryID    *string   `db:"provider_gallery_id"     json:"provider_gallery_id,omitempty"`
	Title                *string   `db:"title"                   json:"title,omitempty"`
	URL                  *string   `db:"url"                     json:"url,omitempty"`
	ThumbnailURL         *string   `db:"thumbnail_url"           json:"thumbnail_url,omitempty"`
	LocalThumbnailPath   *string   `db:"local_thumbnail_path"    json:"local_thumbnail_path,omitempty"`
	Description          *string   `db:"description"             json:"description,omitempty"`
	Rating               *float64  `db:"rating"                  json:"rating,omitempty"`
	ReleaseDate          *string   `db:"release_date"            json:"release_date,omitempty"`
	SourceURL            *string   `db:"source_url"              json:"source_url,omitempty"`
	ProviderThumbnailURL *string   `db:"provider_thumbnail_url"  json:"provider_thumbnail_url,omitempty"`
	CreatedAt            time.Time `db:"created_at"              json:"created_at"`
}

// Image represents a single downloaded image or video file.
type Image struct {
	ID              int64     `db:"id"               json:"id"`
	GalleryID       *int64    `db:"gallery_id"       json:"gallery_id,omitempty"`
	Filename        string    `db:"filename"         json:"filename"`
	OriginalURL     *string   `db:"original_url"     json:"original_url,omitempty"`
	Width           *int      `db:"width"            json:"width,omitempty"`
	Height          *int      `db:"height"           json:"height,omitempty"`
	DurationSeconds *int      `db:"duration_seconds" json:"duration_seconds,omitempty"`
	FileHash        *string   `db:"file_hash"        json:"file_hash,omitempty"`
	DominantColors  *string   `db:"dominant_colors"  json:"dominant_colors,omitempty"`
	IsVideo         bool      `db:"is_video"         json:"is_video"`
	VRMode          string    `db:"vr_mode"          json:"vr_mode"`
	IsFavorite      bool      `db:"is_favorite"      json:"is_favorite"`
	CreatedAt       time.Time `db:"created_at"       json:"created_at"`
}

// Person represents a performer or model profile.
type Person struct {
	ID           int64      `db:"id"           json:"id"`
	Name         string     `db:"name"         json:"name"`
	Aliases      *string    `db:"aliases"      json:"aliases,omitempty"`
	BirthDate    *time.Time `db:"birth_date"   json:"birth_date,omitempty"`
	Nationality  *string    `db:"nationality"  json:"nationality,omitempty"`
	Ethnicity    *string    `db:"ethnicity"    json:"ethnicity,omitempty"`
	HairColor    *string    `db:"hair_color"   json:"hair_color,omitempty"`
	EyeColor     *string    `db:"eye_color"    json:"eye_color,omitempty"`
	Height       *string    `db:"height"       json:"height,omitempty"`
	Weight       *string    `db:"weight"       json:"weight,omitempty"`
	Measurements *string    `db:"measurements" json:"measurements,omitempty"`
	Tattoos      *string    `db:"tattoos"      json:"tattoos,omitempty"`
	Piercings    *string    `db:"piercings"    json:"piercings,omitempty"`
	Biography    *string    `db:"biography"    json:"biography,omitempty"`
	Photos       *string    `db:"photos"       json:"photos,omitempty"` // JSON array of local photo paths
	CreatedAt    time.Time  `db:"created_at"   json:"created_at"`
}

// PersonIdentifier maps a person to an external provider's identifier (e.g. StashDB ID).
type PersonIdentifier struct {
	ID         int64     `db:"id"          json:"id"`
	PersonID   int64     `db:"person_id"   json:"person_id"`
	Provider   string    `db:"provider"    json:"provider"`
	ExternalID string    `db:"external_id" json:"external_id"`
	CreatedAt  time.Time `db:"created_at"  json:"created_at"`
}

// GalleryPerson is the join record linking a gallery to a person.
type GalleryPerson struct {
	GalleryID int64 `db:"gallery_id" json:"gallery_id"`
	PersonID  int64 `db:"person_id"  json:"person_id"`
}

// Tag represents a content tag (e.g. "brunette", "outdoor").
type Tag struct {
	ID       int64   `db:"id"       json:"id"`
	Name     string  `db:"name"     json:"name"`
	Category *string `db:"category" json:"category,omitempty"`
}

// ImageTag links an image to a tag with an optional confidence score (e.g. from AI).
type ImageTag struct {
	ImageID    int64    `db:"image_id"   json:"image_id"`
	TagID      int64    `db:"tag_id"     json:"tag_id"`
	Confidence *float64 `db:"confidence" json:"confidence,omitempty"`
}

// DownloadQueue represents a pending, active, or failed download task.
type DownloadQueue struct {
	ID           int64     `db:"id"            json:"id"`
	Type         string    `db:"type"          json:"type"`
	URL          string    `db:"url"           json:"url"`
	TargetID     *int64    `db:"target_id"     json:"target_id,omitempty"`
	Status       string    `db:"status"        json:"status"`
	RetryCount   int       `db:"retry_count"   json:"retry_count"`
	ErrorMessage *string   `db:"error_message" json:"error_message,omitempty"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`

	// Joined fields (populated by ListQueue with JOINs, not stored directly).
	GalleryTitle *string `db:"gallery_title" json:"gallery_title,omitempty"`
	SourceID     *int64  `db:"source_id"     json:"source_id,omitempty"`
	SourceName   *string `db:"source_name"   json:"source_name,omitempty"`
}

// QueueStatus enumerates valid values for DownloadQueue.Status.
type QueueStatus string

const (
	QueueStatusPending   QueueStatus = "pending"
	QueueStatusActive    QueueStatus = "active"
	QueueStatusCompleted QueueStatus = "completed"
	QueueStatusFailed    QueueStatus = "failed"
	QueueStatusPaused    QueueStatus = "paused"
)

// QueueType enumerates valid values for DownloadQueue.Type.
type QueueType string

const (
	QueueTypeImage   QueueType = "image"
	QueueTypeVideo   QueueType = "video"
	QueueTypeGallery QueueType = "gallery"
	QueueTypeCrawl   QueueType = "crawl"
)

// VRMode enumerates valid values for Image.VRMode.
type VRMode string

const (
	VRModeNone VRMode = "none"
	VRMode180  VRMode = "180"
	VRMode360  VRMode = "360"
)
