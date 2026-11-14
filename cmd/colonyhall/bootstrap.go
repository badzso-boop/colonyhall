// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/config"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/server"
)

// errNotEmpty refuses a bootstrap on a database that is already in use.
var errNotEmpty = errors.New("bootstrap: the database is not empty; it only works on a fresh database " +
	"(use set-password to change a password)")

// bootstrapCmd creates the first workspace and its owner on an empty
// database, for servers that run without the development seed:
//
//	read -rs PW && echo "$PW" | colonyhall bootstrap -handle norbi -name Norbi
//
// The password comes from stdin so it never appears in shell history or
// process lists. The owner's API token is printed once, to stderr.
func bootstrapCmd(args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	workspace := fs.String("workspace", "colonyhall", "workspace name")
	slug := fs.String("slug", "", "the workspace's subdomain name in multi-tenant mode (default: from the name)")
	handle := fs.String("handle", "", "the owner's @handle")
	name := fs.String("name", "", "the owner's display name (default: the handle)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *handle == "" {
		return errors.New("bootstrap: -handle is required")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	pw, err := readPassword(stdin)
	if err != nil {
		return err
	}
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
	token, err := bootstrap(ctx, app, *workspace, *slug, *handle, *name, pw)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "workspace %q created; owner @%s can log in with the password.\n"+
		"Owner API token (shown once; keep it in a password manager or ignore it):\n%s\n", *workspace, *handle, token)
	return nil
}

// bootstrap creates the workspace and the owner with a password on an
// empty database and returns the owner's API token.
func bootstrap(ctx context.Context, app *server.App, workspace, slug, handle, name, password string) (string, error) {
	if err := identity.ValidatePassword(password); err != nil {
		return "", err
	}
	// Every state change writes an event, so an empty log is an empty database.
	head, err := app.Events.Head(ctx)
	if err != nil {
		return "", err
	}
	if head > 0 {
		return "", errNotEmpty
	}
	if name == "" {
		name = handle
	}
	_, owner, token, err := app.Identity.Bootstrap(ctx, workspace, slug, handle, name, "")
	if err != nil {
		return "", err
	}
	if err := app.Identity.ResetPassword(ctx, owner.ID, password); err != nil {
		return "", err
	}
	return token, nil
}

func readPassword(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", err
	}
	pw, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimRight(pw, "\r"), nil
}
