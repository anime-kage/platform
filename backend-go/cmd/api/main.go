// The anime-kage product API: catalog, lists, comments, users.
// Configuration comes from the environment (see internal/config).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"animekage/backend/internal/auth"
	"animekage/backend/internal/config"
	"animekage/backend/internal/db"
	"animekage/backend/internal/handler"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // optional .env, like the old backend

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	slog.Info("database connected")

	h := handler.New(pool, auth.NewManager(cfg.JWTSecret, cfg.JWTExpiresIn), cfg)

	// staging janitor: abandoned release drafts expire after 30
	// days — staging is transient by design, disk usage must trend to zero
	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			h.ExpireStaleStaging(ctx)
			h.PurgeOldChat(ctx) // same cadence, same idea: nothing lives forever
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	// The published-video grace period is measured in minutes, so it needs a
	// ticker to match — the 12-hourly janitor above would make a "5 minute"
	// window mean "up to 12 hours" in practice.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			h.PurgePublishedVideo(ctx)
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Optional hardsub burns. One at a time, niced: a burn saturates
	// all four cores for ~13 minutes per episode, and the API shares them.
	// Requeues anything a previous process left mid-encode.
	h.StartHardsubWorker(ctx)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("anime-kage API listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
