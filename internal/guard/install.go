package guard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// GenerateUnit returns the systemd user service unit definition for colony guard.
// path, when given, becomes the service's PATH: systemd user services start
// with a minimal one (no ~/.local/bin), where the claude and agy CLIs are
// usually installed, so the guard would not find them.
func GenerateUnit(binPath, configPath string, path ...string) (string, error) {
	if binPath == "" {
		return "", fmt.Errorf("colony binary path is required")
	}
	if configPath == "" {
		return "", fmt.Errorf("guard config path is required")
	}
	absBin, err := filepath.Abs(binPath)
	if err != nil {
		return "", fmt.Errorf("resolving colony binary path: %w", err)
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("resolving config path: %w", err)
	}
	workDir := filepath.Dir(absConfig)
	env := ""
	if len(path) > 0 && strings.TrimSpace(path[0]) != "" {
		env = "Environment=" + systemdQuote("PATH="+path[0]) + "\n"
	}

	unit := fmt.Sprintf(`[Unit]
Description=Colonyhall Guard
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
%sExecStart=%s guard -config %s
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`, systemdQuote(workDir), env, systemdQuote(absBin), systemdQuote(absConfig))

	return unit, nil
}

// systemdQuote quotes a path with double quotes if it contains spaces, tabs,
// newlines, or quotes, escaping inner backslashes and double quotes.
func systemdQuote(s string) string {
	// systemd expands %-specifiers and $VARIABLES in these lines; a path
	// containing them must be escaped or it changes meaning.
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, "$", "$$")
	if strings.ContainsAny(s, " \t\r\n\"") {
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// DefaultUnitPath returns ~/.config/systemd/user/colony-guard.service.
func DefaultUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", "colony-guard.service"), nil
}

// Install validates the config, generates a systemd user unit, and either prints
// it to out or writes it to ~/.config/systemd/user/colony-guard.service.
func Install(configPath string, printOnly, force bool, out io.Writer) error {
	if _, err := LoadConfig(configPath); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving colony binary: %w", err)
	}
	// The installing shell's PATH, so the service finds the same CLIs.
	unit, err := GenerateUnit(binPath, configPath, os.Getenv("PATH"))
	if err != nil {
		return err
	}
	if printOnly {
		if out == nil {
			out = os.Stdout
		}
		_, err := fmt.Fprint(out, unit)
		return err
	}

	unitPath, err := DefaultUnitPath()
	if err != nil {
		return fmt.Errorf("resolving unit path: %w", err)
	}
	if _, err := os.Stat(unitPath); err == nil && !force {
		return fmt.Errorf("unit file already exists at %s (use -force to overwrite)", unitPath)
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return fmt.Errorf("creating systemd user directory: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	if out == nil {
		out = os.Stdout
	}
	msg := fmt.Sprintf("Systemd user unit written to %s\n\nEnable and start it with:\n  systemctl --user daemon-reload && systemctl --user enable --now colony-guard\n\nTo keep it running without a login:\n  loginctl enable-linger $USER\n", unitPath)
	_, _ = io.WriteString(out, msg)
	return nil
}
