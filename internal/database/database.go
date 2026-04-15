// Package database manages the SQLite connection pool and schema migrations.
package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/carlj/godownload/internal/config"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

//go:embed migrations
var migrationsFS embed.FS

// DB wraps sqlx.DB and provides migration support.
type DB struct {
	*sqlx.DB
}

// Open opens the SQLite database at the path specified in cfg, configures
// the connection pool, enables WAL mode and foreign keys, and runs all
// pending migrations.
func Open(cfg config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=30000", cfg.Path)

	sqlxDB, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: opening %q: %w", cfg.Path, err)
	}

	sqlxDB.SetMaxOpenConns(1) // SQLite requires single connection
	sqlxDB.SetMaxIdleConns(1)
	sqlxDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlxDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlxDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database: ping failed: %w", err)
	}

	db := &DB{DB: sqlxDB}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("database: migration: %w", err)
	}

	return db, nil
}

// migrate applies any SQL migration files that have not yet been run.
// Migrations are tracked in the schema_migrations table.
func (db *DB) migrate() error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT NOT NULL PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	applied, err := db.appliedMigrations()
	if err != nil {
		return fmt.Errorf("reading applied migrations: %w", err)
	}

	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("listing migration files: %w", err)
	}
	sort.Strings(files)

	for _, file := range files {
		version := migrationVersion(file)
		if applied[version] {
			continue
		}

		slog.Info("applying migration", "version", version)

		content, err := migrationsFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading migration %q: %w", file, err)
		}

		if err := db.runMigration(version, string(content)); err != nil {
			return fmt.Errorf("running migration %q: %w", version, err)
		}
	}

	return nil
}

func (db *DB) appliedMigrations() (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("querying schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scanning migration version: %w", err)
		}
		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating schema_migrations: %w", err)
	}

	return applied, nil
}

func (db *DB) runMigration(version, sql string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback on failure is intentional

	if _, err := tx.Exec(sql); err != nil {
		return fmt.Errorf("executing sql: %w", err)
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version) VALUES (?)", version,
	); err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}

	return nil
}

// migrationVersion extracts the base filename without extension from a path
// like "migrations/001_initial.sql" → "001_initial".
func migrationVersion(path string) string {
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}
	return strings.TrimSuffix(base, ".sql")
}

// ErrNotFound is returned when a query finds no matching row.
var ErrNotFound = errors.New("database: not found")

// IsNotFound reports whether err is ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows)
}
