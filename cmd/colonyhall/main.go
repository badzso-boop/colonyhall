// Command colonyhall runs the Colonyhall server.
//
// Configuration comes from COLONY_* environment variables; see
// internal/platform/config.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"colonyhall/internal/platform/config"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/server"
)

// Usage:
//
//	colonyhall                                   run the server
//	colonyhall set-password -handle norbi        set a password; reads it from stdin
//	colonyhall bootstrap -handle norbi           first workspace + owner on an empty database; password from stdin
//	colonyhall set-slug -workspace W -slug S     rename a workspace's subdomain (multi-tenant mode)
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	var err error
	switch {
	case len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version"):
		fmt.Printf("colonyhall %s (%s/%s)\n", server.Version, runtime.GOOS, runtime.GOARCH)
		return
	case len(os.Args) > 1 && os.Args[1] == "set-password":
		err = setPassword(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "bootstrap":
		err = bootstrapCmd(os.Args[2:], os.Stdin)
	case len(os.Args) > 1 && os.Args[1] == "set-slug":
		err = setSlug(os.Args[2:])
	default:
		err = run(logger)
	}
	if err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// setPassword sets a human's password from the command line. The password
// is read from stdin so it never appears in shell history or process lists:
//
//	read -rs PW && echo "$PW" | colonyhall set-password -handle norbi
func setPassword(args []string) error {
	fs := flag.NewFlagSet("set-password", flag.ContinueOnError)
	handle := fs.String("handle", "", "the person's @handle")
	workspace := fs.String("workspace", "", "workspace name, if the handle exists in several")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *handle == "" {
		return errors.New("set-password: -handle is required")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pw, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	pw = strings.TrimRight(pw, "\r\n")
	ctx := system.Context(context.Background()) // a server command, for any workspace
	app, err := server.New(ctx, server.Options{
		Store:       cfg.Store,
		DatabaseURL: cfg.DatabaseURL,
		FilesDir:    cfg.FilesDir,
		MaxUploadMB: cfg.MaxUploadMB,
	})
	if err != nil {
		return err
	}
	defer app.Close()
	a, err := app.Identity.FindHuman(ctx, *handle, *workspace)
	if err != nil {
		return err
	}
	if err := app.Identity.ResetPassword(ctx, a.ID, pw); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "password set for @%s\n", a.Handle)
	return nil
}

// setSlug renames a workspace's subdomain:
//
//	colonyhall set-slug -workspace colonyhall -slug acme
func setSlug(args []string) error {
	fs := flag.NewFlagSet("set-slug", flag.ContinueOnError)
	workspace := fs.String("workspace", "colonyhall", "workspace name")
	slug := fs.String("slug", "", "the new subdomain name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx := system.Context(context.Background()) // a server command, for any workspace
	app, err := server.New(ctx, server.Options{Store: cfg.Store, DatabaseURL: cfg.DatabaseURL, FilesDir: cfg.FilesDir, MaxUploadMB: cfg.MaxUploadMB})
	if err != nil {
		return err
	}
	defer app.Close()
	ws, err := app.Identity.SetWorkspaceSlug(ctx, *workspace, *slug)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "workspace %q now has the slug %q\n", ws.Name, ws.Slug)
	return nil
}

func run(logger *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := server.New(ctx, server.Options{
		Store:              cfg.Store,
		DatabaseURL:        cfg.DatabaseURL,
		Logger:             logger,
		SecureCookies:      cfg.SecureCookies,
		FilesDir:           cfg.FilesDir,
		MaxUploadMB:        cfg.MaxUploadMB,
		CalendarRefresh:    cfg.CalendarRefresh,
		GiteaURL:           cfg.GiteaURL,
		GiteaToken:         cfg.GiteaToken,
		GiteaPublicURL:     cfg.GiteaPublicURL,
		GiteaWebhookSecret: cfg.GiteaWebhookSecret,
	})
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

	// Per-request read and write deadlines are set by the handler (they must
	// spare the WebSockets and the MCP stream); the server bounds the rest.
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: app.Handler, ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10}
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
