// Package main is the entrypoint for the GoDownload API server.
// It loads configuration, initialises dependencies, registers routes,
// and starts the HTTP server with graceful shutdown.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/carlj/godownload/internal/config"
	"github.com/carlj/godownload/internal/database"
	"github.com/carlj/godownload/internal/handlers"
	"github.com/carlj/godownload/internal/middleware"
	"github.com/carlj/godownload/internal/services/crawler"
	"github.com/carlj/godownload/internal/services/linker"
	"github.com/carlj/godownload/internal/services/providers"
	"github.com/carlj/godownload/internal/services/queue"
	"github.com/carlj/godownload/internal/services/queue/processors"
	"github.com/carlj/godownload/internal/services/ripper"
	ripperproviders "github.com/carlj/godownload/internal/services/ripper/providers"
	"github.com/carlj/godownload/internal/utils"
	"github.com/gin-gonic/gin"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.Log)

	db, err := database.Open(cfg.Database)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	crawlerSvc := crawler.New(db, cfg.Crawler)
	crawlerSvc.RegisterParser(crawler.NewViperGirls())
	crawlerSvc.RegisterParser(crawler.NewJKForum())
	crawlerSvc.RegisterParser(crawler.NewKittyKats())
	defer crawlerSvc.Stop()

	// Build a shared HTTP client and ripper registry.
	httpClient := utils.NewHTTPClient(utils.WithTimeout(cfg.Crawler.RequestTimeout))
	ripperReg := ripper.NewRegistry(
		cfg.Storage.ImagesDir,
		httpClient,
		ripper.WithUserAgent(cfg.Crawler.UserAgent),
	)
	ripperproviders.RegisterAll(ripperReg, httpClient, cfg.Crawler.UserAgent)

	enricher := providers.NewEnricher(httpClient, cfg.Crawler.UserAgent, "")

	queueMgr := queue.New(db, cfg.Crawler.Workers, cfg.Crawler.MaxRetries)
	processors.New(db, ripperReg, *cfg).Register(queueMgr)
	queueMgr.Start()
	defer queueMgr.Stop()

	autoLinker := linker.New(db)

	router := buildRouter(db, crawlerSvc, autoLinker, enricher)

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start the server in a goroutine so we can listen for shutdown signals.
	go func() {
		slog.Info("server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Block until a termination signal is received.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// buildRouter wires up all routes and returns the configured gin.Engine.
func buildRouter(db *database.DB, crawlerSvc *crawler.Crawler, al *linker.AutoLinker, enricher *providers.Enricher) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())

	v1 := r.Group("/api/v1")

	handlers.NewSourceHandler(db, crawlerSvc).RegisterRoutes(v1.Group("/sources"))
	handlers.NewGalleryHandler(db).RegisterRoutes(v1.Group("/galleries"))
	handlers.NewImageHandler(db).RegisterRoutes(v1.Group("/images"))
	handlers.NewVideoHandler(db).RegisterRoutes(v1.Group("/videos"))
	handlers.NewPeopleHandler(db, al, enricher).RegisterRoutes(v1.Group("/people"))
	handlers.NewAdminHandler(db).RegisterRoutes(v1.Group("/admin"))

	// Health check endpoint.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}

// setupLogger configures the global slog logger based on config.
func setupLogger(cfg config.LogConfig) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
