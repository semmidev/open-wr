package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/semmidev/wr/internal/api"
	"github.com/semmidev/wr/internal/config"
	"github.com/semmidev/wr/internal/cookie"
	"github.com/semmidev/wr/internal/proxy"
	"github.com/semmidev/wr/internal/room"
	"github.com/semmidev/wr/internal/waitingroom"
)

const (
	shutdownTimeout = 10 * time.Second
	serviceName     = "open-wr edge"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "config.example.yaml", "path to config YAML file")
	showVersion := flag.Bool("version", false, "print version information and exit")
	healthcheck := flag.Bool("healthcheck", false, "perform internal healthcheck request to localhost and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s version %s (commit: %s, built: %s)\n", serviceName, version, commit, buildTime)
		os.Exit(0)
	}

	if *healthcheck {
		port := ":8080"
		if cfg, err := config.Load(*configPath); err == nil && cfg.Server.Listen != "" {
			port = cfg.Server.Listen
		}
		if len(port) > 0 && port[0] == ':' {
			port = "127.0.0.1" + port
		}
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + port + "/health") // #nosec G704 -- internal container healthcheck request
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// --- Config ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: load config %q: %v\n", *configPath, err)
		os.Exit(1)
	}

	// --- Logger ---
	logger := buildLogger(cfg.Server.LogLevel)
	logger.Info("starting "+serviceName,
		"version", version,
		"commit", commit,
		"build_time", buildTime,
		"listen", cfg.Server.Listen,
		"origin", cfg.Server.Origin,
		"rooms", len(cfg.Rooms),
		"redis", cfg.Server.RedisEnabled,
		"secure_cookies", cfg.Server.SecureCookies,
	)

	// Warn if admin API is unprotected.
	if cfg.Server.AdminAPIKey == "" {
		logger.Warn("admin API is unprotected — set admin_api_key in config YAML file for production")
	}

	// --- Store ---
	store := buildStore(cfg, logger)

	// --- Cookie signer ---
	signer, err := cookie.NewSigner(cfg.Server.CookieSecret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: cookie signer: %v\n", err)
		os.Exit(1)
	}

	// --- Waiting room service ---
	wrService := waitingroom.NewService(cfg, store, signer, logger)

	// --- Origin proxy ---
	var originProxy *proxy.ReverseProxy
	if cfg.Server.Origin != "" {
		originProxy, err = proxy.NewReverseProxy(cfg.Server.Origin, logger)
		if err != nil {
			logger.Warn("origin proxy disabled — invalid origin URL", "origin", cfg.Server.Origin, "err", err)
		}
	}
	demoOrigin := proxy.DemoOriginHandler()

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(api.PoweredByMiddleware(serviceName))

	// Health check — no auth required.
	r.Get("/health", healthHandler)

	// Admin API — protected by API key when configured.
	adminHandler := api.NewAdminHandler(wrService)
	r.Group(func(r chi.Router) {
		r.Use(api.AdminAuthMiddleware(cfg.Server.AdminAPIKey))
		r.Mount("/api/rooms", adminHandler.Routes())
	})

	// Status endpoint for JS polling.
	statusHandler := api.NewStatusHandler(wrService, signer, cfg.Server.CORSOrigin)
	r.Handle("/__owr/waiting-room/status", statusHandler)
	r.Handle("/__owr/waiting-room/status/", statusHandler)

	// Waiting room catch-all.
	wrHandler := api.NewWaitingRoomHandler(api.WaitingRoomHandlerConfig{
		Rooms:         cfg.Rooms,
		Service:       wrService,
		Store:         store,
		Signer:        signer,
		OriginProxy:   originProxy,
		DemoOrigin:    demoOrigin,
		SecureCookies: cfg.Server.SecureCookies,
	})
	r.Handle("/*", wrHandler)

	// --- Admission workers ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, rc := range cfg.Rooms {
		if !rc.Enabled {
			continue
		}
		worker := waitingroom.NewAdmissionWorker(rc, store, logger)
		go worker.Start(ctx)
	}

	// --- HTTP server ---
	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info(serviceName+" listening", "addr", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down gracefully...")
	cancel() // stop admission workers

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}

// buildLogger creates a slog.Logger at the configured level.
func buildLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

// buildStore constructs the appropriate room.Store based on config.
func buildStore(cfg *config.Config, logger *slog.Logger) room.Store {
	if cfg.Server.RedisEnabled {
		logger.Info("using Redis store", "addr", cfg.Server.RedisAddr)
		return room.NewRedisStore(cfg.Server.RedisAddr)
	}
	logger.Info("using in-memory store (single-instance only)")
	return room.NewMemStore()
}

// healthHandler serves a simple JSON health response.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","service":"` + serviceName + `"}`))
}
