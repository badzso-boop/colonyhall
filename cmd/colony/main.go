// Command colony is the client-side tool of Colonyhall. `colony guard`
// watches agents' inboxes and wakes their AI clients (Claude Code,
// Antigravity, …) when something arrives.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"colonyhall/internal/guard"
)

const usage = `usage:
  colony guard enroll -server URL -code CODE -name NAME [-token-file PATH]
      pair this machine (the admin creates the code); stores the host token
  colony guard install -config PATH [-print] [-force]
      install a systemd user unit to ~/.config/systemd/user/colony-guard.service
  colony guard [-config guard.yaml]
      run the guard (host mode when host_token_file is set)

clients: %s
`

func main() {
	if len(os.Args) > 1 && os.Args[1] == "sandbox-bridge" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runSandboxBridge(ctx, os.Args[2:]); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "colony sandbox-bridge: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "terminal-mcp" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runTerminalMCP(ctx, os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "colony terminal-mcp: %v\n", err)
			os.Exit(1)
		}
		return
	}
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
	if len(os.Args) > 2 && os.Args[2] == "install" {
		if err := install(os.Args[3:]); err != nil {
			log.Error("colony guard install", "err", err)
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

func runTerminalMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("terminal-mcp", flag.ExitOnError)
	agent := fs.String("agent", "", "agent handle")
	stateDir := fs.String("state", "", "state directory")
	channelID := fs.String("channel", "", "channel ID")
	threadRootID := fs.String("thread", "", "thread root ID")
	sandbox := fs.Bool("sandbox", false, "whether agent is sandboxed")
	_ = fs.Parse(args)

	if *sandbox {
		return errors.New("sandboxed agents may not run terminals")
	}
	if *agent == "" || *stateDir == "" {
		return errors.New("-agent and -state are required")
	}

	return guard.RunTerminalMCPServer(ctx, *agent, *stateDir, *channelID, *threadRootID)
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

func install(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	config := fs.String("config", "", "path to guard config file")
	printOnly := fs.Bool("print", false, "write unit to stdout instead of installing")
	force := fs.Bool("force", false, "overwrite existing unit file")
	_ = fs.Parse(args)
	if *config == "" {
		return fmt.Errorf("-config is required")
	}
	return guard.Install(*config, *printOnly, *force, os.Stdout)
}

func runSandboxBridge(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sandbox-bridge", flag.ContinueOnError)
	sock := fs.String("sock", "", "path to unix domain socket")
	port := fs.Int("port", 3128, "local TCP port to listen on")
	webreadSock := fs.String("webread-sock", "", "path to webread unix domain socket")
	webreadPort := fs.Int("webread-port", 3129, "local TCP port for webread")
	plannerSock := fs.String("planner-sock", "", "path to planner unix domain socket")
	plannerPort := fs.Int("planner-port", 3130, "local TCP port for planner")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *sock == "" || len(rest) == 0 {
		return fmt.Errorf("usage: colony sandbox-bridge -sock <path> [-port <port>] [-webread-sock <path>] [-webread-port <port>] [-planner-sock <path>] [-planner-port <port>] -- <client> <args...>")
	}
	mappings := []guard.BridgeMapping{
		{SockPath: *sock, Port: *port},
	}
	if *webreadSock != "" {
		mappings = append(mappings, guard.BridgeMapping{SockPath: *webreadSock, Port: *webreadPort})
	}
	if *plannerSock != "" {
		mappings = append(mappings, guard.BridgeMapping{SockPath: *plannerSock, Port: *plannerPort})
	}
	return guard.RunMultiBridgeCommand(ctx, mappings, rest[0], rest[1:])
}
