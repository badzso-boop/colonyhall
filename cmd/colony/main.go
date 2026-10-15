// Command colony is the client-side tool of Colonyhall. `colony guard`
// watches agents' inboxes and wakes their AI clients (Claude Code,
// Antigravity, …) when something arrives.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"colonyhall/internal/guard"
)

const usage = `usage:
  colony guard enroll -server URL -code CODE -name NAME [-token-file PATH]
      pair this machine (the admin creates the code); stores the host token
  colony guard [-config guard.yaml]
      run the guard (host mode when host_token_file is set)

clients: %s
`

func main() {
	if len(os.Args) < 2 || os.Args[1] != "guard" {
		fmt.Fprintf(os.Stderr, usage, strings.Join(guard.AdapterNames(), ", "))
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 2 && os.Args[2] == "enroll" {
		if err := enroll(ctx, os.Args[3:]); err != nil {
			log.Error("colony guard enroll", "err", err)
			os.Exit(1)
		}
		return
	}
	fs := flag.NewFlagSet("guard", flag.ExitOnError)
	path := fs.String("config", "guard.yaml", "guard config file")
	_ = fs.Parse(os.Args[2:])

	cfg, err := guard.LoadConfig(*path)
	if err != nil {
		log.Error("colony guard", "err", err)
		os.Exit(1)
	}
	if cfg.HostMode() {
		err = (&guard.Supervisor{Cfg: cfg, Run: guard.ExecRunner, Log: log}).RunHost(ctx)
	} else {
		err = guard.RunAll(ctx, cfg, guard.ExecRunner, log)
	}
	if err != nil {
		log.Error("colony guard", "err", err)
		os.Exit(1)
	}
}

func enroll(ctx context.Context, args []string) error {
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	server := fs.String("server", "", "Colonyhall server URL")
	code := fs.String("code", "", "pairing code from the admin")
	name := fs.String("name", "", "a name for this machine")
	tokenFile := fs.String("token-file", home+"/.config/colonyhall/host.token", "where to store the host token (0600)")
	_ = fs.Parse(args)
	if *server == "" || *code == "" || *name == "" {
		return fmt.Errorf("-server, -code and -name are required")
	}
	if err := guard.Enroll(ctx, *server, *code, *name, *tokenFile); err != nil {
		return err
	}
	fmt.Printf("Enrolled as %q. Host token saved to %s.\nIn guard.yaml set: host_token_file: %s\n", *name, *tokenFile, *tokenFile)
	return nil
}
