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

func main() {
	if len(os.Args) < 2 || os.Args[1] != "guard" {
		fmt.Fprintf(os.Stderr, "usage: colony guard -config guard.yaml\n\nclients: %s\n", strings.Join(guard.AdapterNames(), ", "))
		os.Exit(2)
	}
	fs := flag.NewFlagSet("guard", flag.ExitOnError)
	path := fs.String("config", "guard.yaml", "guard config file")
	_ = fs.Parse(os.Args[2:])

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := guard.LoadConfig(*path)
	if err != nil {
		log.Error("colony guard", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := guard.RunAll(ctx, cfg, guard.ExecRunner, log); err != nil {
		log.Error("colony guard", "err", err)
		os.Exit(1)
	}
}
