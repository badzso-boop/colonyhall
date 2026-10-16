package guard

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateUnit(t *testing.T) {
	t.Run("standard paths without spaces", func(t *testing.T) {
		unit, err := GenerateUnit("/usr/local/bin/colony", "/home/me/colonyhall/guard.yaml")
		if err != nil {
			t.Fatalf("GenerateUnit failed: %v", err)
		}
		if !strings.Contains(unit, "WorkingDirectory=/home/me/colonyhall\n") {
			t.Errorf("expected unquoted WorkingDirectory=/home/me/colonyhall, got:\n%s", unit)
		}
		if !strings.Contains(unit, "ExecStart=/usr/local/bin/colony guard -config /home/me/colonyhall/guard.yaml\n") {
			t.Errorf("expected unquoted ExecStart, got:\n%s", unit)
		}
		if !strings.Contains(unit, "Restart=on-failure\nRestartSec=10\n") {
			t.Errorf("expected Restart settings, got:\n%s", unit)
		}
		if !strings.Contains(unit, "WantedBy=default.target\n") {
			t.Errorf("expected WantedBy=default.target, got:\n%s", unit)
		}
	})

	t.Run("paths with spaces quoted correctly", func(t *testing.T) {
		unit, err := GenerateUnit("/opt/my tools/colony", "/mnt/g/My Projects/colony/guard.yaml")
		if err != nil {
			t.Fatalf("GenerateUnit failed: %v", err)
		}
		if !strings.Contains(unit, `WorkingDirectory="/mnt/g/My Projects/colony"`+"\n") {
			t.Errorf("expected quoted WorkingDirectory, got:\n%s", unit)
		}
		if !strings.Contains(unit, `ExecStart="/opt/my tools/colony" guard -config "/mnt/g/My Projects/colony/guard.yaml"`+"\n") {
			t.Errorf("expected quoted ExecStart, got:\n%s", unit)
		}
	})

	t.Run("paths with inner quotes escaped", func(t *testing.T) {
		unit, err := GenerateUnit(`/opt/my"tools/colony`, `/home/me/my"dir/guard.yaml`)
		if err != nil {
			t.Fatalf("GenerateUnit failed: %v", err)
		}
		if !strings.Contains(unit, `WorkingDirectory="/home/me/my\"dir"`+"\n") {
			t.Errorf("expected escaped quotes in WorkingDirectory, got:\n%s", unit)
		}
		if !strings.Contains(unit, `ExecStart="/opt/my\"tools/colony" guard -config "/home/me/my\"dir/guard.yaml"`+"\n") {
			t.Errorf("expected escaped quotes in ExecStart, got:\n%s", unit)
		}
	})

	t.Run("empty paths error", func(t *testing.T) {
		if _, err := GenerateUnit("", "/path/guard.yaml"); err == nil {
			t.Error("expected error for empty binPath")
		}
		if _, err := GenerateUnit("/bin/colony", ""); err == nil {
			t.Error("expected error for empty configPath")
		}
	})
}

func TestInstallPrint(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "guard.yaml")
	cfgContent := `server: http://localhost:8080
host_token_file: ~/.config/colonyhall/host.token
roots:
  - /tmp
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := Install(cfgPath, true, false, &buf)
	if err != nil {
		t.Fatalf("Install with printOnly failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[Unit]") || !strings.Contains(out, "Description=Colonyhall Guard") {
		t.Errorf("unexpected output:\n%s", out)
	}
	if !strings.Contains(out, cfgPath) {
		t.Errorf("expected config path %s in output:\n%s", cfgPath, out)
	}
}

func TestInstallInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(cfgPath, []byte("server: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := Install(cfgPath, true, false, &buf)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestInstallFileAndForce(t *testing.T) {
	dir := t.TempDir()
	// Override HOME so DefaultUnitPath uses our temp dir
	t.Setenv("HOME", dir)

	cfgPath := filepath.Join(dir, "guard.yaml")
	cfgContent := `server: http://localhost:8080
host_token_file: ~/.config/colonyhall/host.token
roots:
  - /tmp
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}

	unitPath, err := DefaultUnitPath()
	if err != nil {
		t.Fatal(err)
	}

	// 1. First install: writes unit file.
	var buf bytes.Buffer
	if err := Install(cfgPath, false, false, &buf); err != nil {
		t.Fatalf("initial Install failed: %v", err)
	}
	content, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit file was not created: %v", err)
	}
	if !strings.Contains(string(content), "Description=Colonyhall Guard") {
		t.Errorf("unexpected unit file content:\n%s", string(content))
	}
	if !strings.Contains(buf.String(), "systemctl --user daemon-reload && systemctl --user enable --now colony-guard") {
		t.Errorf("missing instructions in output:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "loginctl enable-linger $USER") {
		t.Errorf("missing linger instruction in output:\n%s", buf.String())
	}

	// 2. Second install without force: must error out.
	buf.Reset()
	if err := Install(cfgPath, false, false, &buf); err == nil {
		t.Fatal("expected error when unit file already exists without -force")
	}

	// 3. Third install with force: must succeed and overwrite.
	buf.Reset()
	if err := Install(cfgPath, false, true, &buf); err != nil {
		t.Fatalf("Install with force failed: %v", err)
	}
}
