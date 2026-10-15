// Command colonyhall runs the Colonyhall server.
//
// Configuration comes from COLONY_* environment variables; see
// internal/platform/config.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"colonyhall/internal/platform/config"
	"colonyhall/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := server.New(ctx, server.Options{Store: cfg.Store, DatabaseURL: cfg.DatabaseURL, Logger: logger})
	if err != nil {
		return err
	}
	defer app.Close()

	if cfg.DevSeed {
		if err := app.SeedDev(ctx); err != nil {
			return err
		}
		logger.Warn("development seed active: fixed tokens, do not expose this server",
			"admin_token", server.DevAdminToken, "agent_token", server.DevAgentToken)
	}

	go func() {
		if err := app.Run(ctx); err != nil {
			logger.Error("background workers stopped", "err", err)
		}
	}()

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: app.Handler, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	logger.Info("colonyhall listening", "addr", cfg.HTTPAddr, "store", cfg.Store, "version", server.Version)

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger.Info("shutting down")
	return srv.Shutdown(shutdownCtx)
}
