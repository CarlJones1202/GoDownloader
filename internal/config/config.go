// Package config loads and validates application configuration from a YAML file.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Crawler   CrawlerConfig   `yaml:"crawler"`
	Queue     QueueConfig     `yaml:"queue"`
	Providers ProvidersConfig `yaml:"providers"`
	Storage   StorageConfig   `yaml:"storage"`
	Log       LogConfig       `yaml:"log"`
	WireGuard WireGuardConfig `yaml:"wireguard"`
}

// ProvidersConfig holds API keys and provider-specific settings.
type ProvidersConfig struct {
	StashDBAPIKey string `yaml:"stashdb_api_key"`
}

// QueueConfig holds download-queue concurrency settings.
// These are intentionally separate from CrawlerConfig because the crawler
// (page fetching) and the downloader (image fetching) have very different
// concurrency needs.
type QueueConfig struct {
	// Workers is the total maximum number of concurrent download goroutines.
	Workers int `yaml:"workers"`
	// ProviderLimit is the maximum number of concurrent downloads per image host.
	ProviderLimit int `yaml:"provider_limit"`
	// ProviderPool is how many pending items are fetched per provider per poll tick.
	ProviderPool int `yaml:"provider_pool"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// DatabaseConfig holds SQLite settings.
type DatabaseConfig struct {
	Path            string        `yaml:"path"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`
}

// CrawlerConfig holds source crawling settings.
type CrawlerConfig struct {
	Workers        int           `yaml:"workers"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
	RateLimit      time.Duration `yaml:"rate_limit"`
	MaxRetries     int           `yaml:"max_retries"`
	UserAgent      string        `yaml:"user_agent"`
	CrawlInterval  time.Duration `yaml:"crawl_interval"`
}

// StorageConfig holds file storage paths.
type StorageConfig struct {
	ImagesDir       string `yaml:"images_dir"`
	ThumbnailsDir   string `yaml:"thumbnails_dir"`
	VideosDir       string `yaml:"videos_dir"`
	PersonPhotosDir string `yaml:"person_photos_dir"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // "json" or "text"
}

// WireGuardConfig holds WireGuard VPN settings for selective domain routing.
type WireGuardConfig struct {
	// ConfPath is the path to a standard WireGuard .conf file.
	ConfPath string `yaml:"conf_path"`
	// Bypass disables VPN routing even when configured (for testing).
	Bypass bool `yaml:"bypass"`
	// BlockedDomains is the list of domains that must be routed through the VPN.
	// If empty, a sensible default list is used.
	BlockedDomains []string `yaml:"blocked_domains"`
}

// Load reads a YAML config file from the given path, applies defaults,
// and validates the result.
func Load(path string) (*Config, error) {
	cfg := defaults()

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: opening file %q: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("config: decoding %q: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: validation: %w", err)
	}

	return cfg, nil
}

// defaults returns a Config pre-populated with sensible defaults.
func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Database: DatabaseConfig{
			Path:            "godownload.db",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 1 * time.Minute,
		},
		Crawler: CrawlerConfig{
			Workers:        5,
			RequestTimeout: 30 * time.Second,
			RateLimit:      500 * time.Millisecond,
			MaxRetries:     3,
			UserAgent:      "GoDownload/1.0",
			CrawlInterval:  6 * time.Hour,
		},
		Storage: StorageConfig{
			ImagesDir:       "data/images",
			ThumbnailsDir:   "data/thumbnails",
			VideosDir:       "data/videos",
			PersonPhotosDir: "data/person_photos",
		},
		Providers: ProvidersConfig{
			StashDBAPIKey: "",
		},
		Queue: QueueConfig{
			Workers:       30, // 30 total concurrent downloads
			ProviderLimit: 10, // up to 10 simultaneous downloads per image host
			ProviderPool:  10, // fetch 10 queued items per provider per poll tick
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// validate checks that required fields have valid values.
func (c *Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d is out of range [1, 65535]", c.Server.Port)
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path must not be empty")
	}
	if c.Crawler.Workers < 1 {
		return fmt.Errorf("crawler.workers must be >= 1")
	}
	return nil
}

// Addr returns the formatted listen address, e.g. "0.0.0.0:8080".
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
