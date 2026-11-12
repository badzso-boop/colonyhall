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

	"colonyhall/internal/modules/git"
	"colonyhall/internal/platform/config"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/server"
)

// Usage:
//
//	colonyhall                                   run the server
//	colonyhall set-password -handle norbi        set a password; reads it from stdin
//	colonyhall bootstrap -handle norbi           first workspace + owner on an empty database; password from stdin
//	colonyhall rag-backfill [-dry-run]           (re)build the search index from the event log
//	colonyhall rag-reembed                       re-embed after an embedding model change
//	colonyhall rag-eval -set FILE [-as HANDLE]   search quality: recall@k, MRR, nDCG, latency
//	colonyhall set-slug -workspace W -slug S     rename a workspace's subdomain (multi-tenant mode)
//	colonyhall gitea-set -workspace W -url U     a workspace's own Gitea; token and webhook secret from stdin
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
	case len(os.Args) > 1 && os.Args[1] == "rag-backfill":
		err = ragBackfill(os.Args[2:], os.Stdout)
	case len(os.Args) > 1 && os.Args[1] == "rag-reembed":
		err = ragReembed(os.Args[2:], os.Stdout)
	case len(os.Args) > 1 && os.Args[1] == "rag-eval":
		err = ragEval(os.Args[2:], os.Stdout)
	case len(os.Args) > 1 && os.Args[1] == "set-slug":
		err = setSlug(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "gitea-set":
		err = giteaSet(os.Args[2:], os.Stdin)
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

// giteaSet gives a workspace its own Gitea instead of the shared one, or
// switches it off (round 45). The read-only token and the webhook secret
// come from stdin, one per line, so they never reach shell history:
//
//	printf '%s\n%s\n' "$TOKEN" "$SECRET" | colonyhall gitea-set -workspace acme -url https://git.acme.example
//	colonyhall gitea-set -workspace acme -off
func giteaSet(args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("gitea-set", flag.ContinueOnError)
	workspace := fs.String("workspace", "", "workspace name")
	url := fs.String("url", "", "the Gitea API address the server calls")
	publicURL := fs.String("public-url", "", "the address people use in links, if different")
	off := fs.Bool("off", false, "switch the own Gitea off (back to the shared one)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workspace == "" || (!*off && *url == "") {
		return errors.New("gitea-set: -workspace and -url (or -off) are required")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	in := git.WorkspaceGitea{Enabled: !*off, URL: *url, PublicURL: *publicURL}
	if !*off {
		r := bufio.NewReader(stdin)
		tok, _ := r.ReadString('\n')
		sec, _ := r.ReadString('\n')
		in.Token, in.WebhookSecret = strings.TrimSpace(tok), strings.TrimSpace(sec)
		if in.Token == "" || in.WebhookSecret == "" {
			return errors.New("gitea-set: give the token and the webhook secret on stdin, one per line")
		}
	}
	ctx := system.Context(context.Background()) // a server command, for any workspace
	app, err := server.New(ctx, server.Options{Store: cfg.Store, DatabaseURL: cfg.DatabaseURL, FilesDir: cfg.FilesDir, MaxUploadMB: cfg.MaxUploadMB})
	if err != nil {
		return err
	}
	defer app.Close()
	ws, err := app.Identity.Store.WorkspaceByName(ctx, *workspace)
	if err != nil {
		return err
	}
	in.WorkspaceID = ws.ID
	if *off {
		// Keep the stored address for a later switch back on.
		if old, err := app.Git.Store.GetWorkspaceGitea(ctx, ws.ID); err == nil {
			in.URL, in.PublicURL, in.Token, in.WebhookSecret = old.URL, old.PublicURL, old.Token, old.WebhookSecret
		}
	}
	if err := app.Git.SetWorkspaceGitea(ctx, in); err != nil {
		return err
	}
	state := "on"
	if *off {
		state = "off"
	}
	fmt.Fprintf(os.Stderr, "own Gitea for %q: %s\n", ws.Name, state)
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
		Multitenant:        cfg.Multitenant,
		BaseDomain:         cfg.BaseDomain,
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
		EmbedURL:           cfg.EmbedURL,
		RerankURL:          cfg.RerankURL,
		RerankTop:          cfg.RerankTop,
		RerankTimeout:      cfg.RerankTimeout,
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
