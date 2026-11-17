// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/config"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/secretbox"
	"colonyhall/internal/server"
)

// secretKey prints a fresh COLONY_SECRET_KEYS entry (round 51). Rotation:
// put the new key first, keep the old one after it, run rotate-secrets,
// then drop the old one.
func secretKey(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("secret-key", flag.ContinueOnError)
	id := fs.String("id", "k1", "the key's id (short, lower case); use a new one for each rotation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	k, err := secretbox.NewKey(*id)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, k)
	return err
}

// operatorCmd runs the operator commands of round 51 (multi-tenant mode):
//
//	read -rs PW && echo "$PW" | colonyhall operator-create -handle ops -name "Ops"
//
// The password comes from stdin's first line; operator-create and
// operator-reset-totp then print the TOTP secret, read the first code from
// the next line and print the backup codes once.
func operatorCmd(cmd string, args []string, stdin io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	handle := fs.String("handle", "", "the operator's handle")
	name := fs.String("name", "", "the operator's display name (operator-create; default: the handle)")
	code := fs.String("code", "", "the authenticator app's 6-digit code (operator-confirm)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cmd != "rotate-secrets" && *handle == "" {
		return fmt.Errorf("%s: -handle is required", cmd)
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if !cfg.Multitenant && cmd != "rotate-secrets" {
		return operator.ErrNotMultitenant
	}
	ctx := system.Context(context.Background()) // a server command
	app, err := server.New(ctx, server.Options{Store: cfg.Store, DatabaseURL: cfg.DatabaseURL, FilesDir: cfg.FilesDir,
		MaxUploadMB: cfg.MaxUploadMB, Multitenant: cfg.Multitenant, BaseDomain: cfg.BaseDomain, SecretKeys: cfg.SecretKeys})
	if err != nil {
		return err
	}
	defer app.Close()
	if cmd == "rotate-secrets" {
		// Round 56a: the workspaces' secrets (connector tokens), on every
		// server; the operators' TOTP secrets in multi-tenant mode.
		n, err := app.Secrets.Rotate(ctx)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "re-encrypted %d workspace secret(s) with the key %q\n", n, cfg.SecretKeys.Active()); err != nil {
			return err
		}
		if !cfg.Multitenant {
			return nil
		}
	}
	in := stdin
	if *code != "" {
		in = strings.NewReader(*code + "\n")
	}
	return runOperatorCmd(context.Background(), app.Operator, cmd, *handle, *name, bufio.NewReader(in), out)
}

func runOperatorCmd(ctx context.Context, svc *operator.Service, cmd, handle, name string, in *bufio.Reader, out io.Writer) error {
	line := func() (string, error) {
		s, err := in.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(s, "\r\n"), nil
	}
	activate := func(code string) error {
		codes, err := svc.ConfirmTOTP(ctx, handle, code)
		if err != nil {
			return fmt.Errorf("%w (try operator-confirm again with a fresh code, or operator-reset-totp)", err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "\n@%s is active. Backup codes (each works once; keep them offline, shown once):\n", handle)
		for _, c := range codes {
			fmt.Fprintf(&b, "  %s\n", c)
		}
		_, err = io.WriteString(out, b.String())
		return err
	}
	confirm := func(en operator.Enrollment) error {
		if _, err := fmt.Fprintf(out, "Add this to the authenticator app (shown once):\n  secret: %s\n  %s\nType the 6-digit code it shows: ", en.Secret, en.URI); err != nil {
			return err
		}
		code, err := line()
		if err != nil {
			return err
		}
		if strings.TrimSpace(code) == "" {
			// No terminal (a pipe, docker exec -T): confirm in a second step.
			_, err := fmt.Fprintf(out, "\nNo code read. Confirm with: colonyhall operator-confirm -handle %s -code <6 digits>\n", handle)
			return err
		}
		return activate(code)
	}
	switch cmd {
	case "operator-create":
		pw, err := line()
		if err != nil {
			return err
		}
		_, en, err := svc.CreateOperator(ctx, handle, name, pw)
		if err != nil {
			return err
		}
		return confirm(en)
	case "operator-reset-totp":
		en, err := svc.ResetTOTP(ctx, handle)
		if err != nil {
			return err
		}
		return confirm(en)
	case "operator-confirm":
		code, err := line()
		if err != nil {
			return err
		}
		return activate(code)
	case "operator-disable":
		if err := svc.Disable(ctx, handle); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "@%s is disabled; their sessions ended.\n", handle)
		return err
	case "rotate-secrets":
		n, err := svc.RotateSecrets(ctx)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "re-encrypted %d secret(s) with the key %q\n", n, svc.Keys.Active())
		return err
	}
	return fmt.Errorf("unknown command %q", cmd)
}
