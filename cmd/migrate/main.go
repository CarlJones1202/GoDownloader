// Package main is the entrypoint for the GoDownload migration tool.
// It opens the database at the configured path and applies any pending migrations.
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("migrations complete")
}
