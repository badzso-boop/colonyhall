package guard

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
)

func newTestAgent(t *testing.T, sandbox bool, roots []string, grants []Grant) (*Agent, string) {
	t.Helper()
	dir := t.TempDir()
	ag := &Agent{
		Cfg:     AgentConfig{Name: "test-bot"},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sandbox: sandbox,
		RootDir: dir,
		Roots:   roots,
	}
	if len(grants) > 0 {
		ag.GrantsFunc = func() []Grant { return grants }
	}
	return ag, dir
}

func TestFsBrowse_ScopeRoots(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()

	// Full-access agent: RootDir is scope root
	agFull, _ := newTestAgent(t, false, []string{d1}, nil)
	roots := agFull.ScopeRoots()
	if len(roots) != 1 || roots[0].Path != d1 || roots[0].Mode != "write" {
		t.Fatalf("expected 1 write root for full access, got: %+v", roots)
	}

	// Sandboxed agent with 2 grants
	agSand, _ := newTestAgent(t, true, nil, []Grant{
		{Path: d1, Mode: "read", Write: false},
		{Path: d2, Mode: "write", Write: true},
	})
	roots = agSand.ScopeRoots()
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots for sandboxed, got: %+v", roots)
	}
	if roots[0].Mode != "read" || roots[1].Mode != "write" {
		t.Fatalf("expected read and write modes, got: %+v", roots)
	}
}

func TestFsBrowse_ListDirectory(t *testing.T) {
	root := t.TempDir()
	ag, _ := newTestAgent(t, false, []string{root}, nil)

	// Create test files and directories
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("# Hello"), 0o644)

	// Hidden elements
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "id_rsa"), []byte("key"), 0o600)
	_ = os.WriteFile(filepath.Join(root, "server.key"), []byte("key"), 0o600)

	// Heavy directory
	_ = os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "node_modules", "pkg", "index.js"), []byte("module.exports={}"), 0o644)

	// Symlinks
	// In-scope symlink
	_ = os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "link_readme.md"))
	// Out-of-scope symlink
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)
	_ = os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link_outside.txt"))

	res, err := ag.ListDirectory(filepath.Base(root), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != nil {
		t.Fatalf("unexpected error code: %s", *res.ErrorCode)
	}
	if res.Entries == nil {
		t.Fatal("expected entries")
	}

	names := make(map[string]apispec.FsEntry)
	for _, e := range *res.Entries {
		names[e.Name] = e
	}

	// 1. Hidden items must not appear
	for _, hidden := range []string{".git", ".env", "id_rsa", "server.key"} {
		if _, ok := names[hidden]; ok {
			t.Errorf("hidden item %q should not appear in listing", hidden)
		}
	}

	// 2. Heavy dir is marked heavy
	if nm, ok := names["node_modules"]; !ok {
		t.Error("node_modules should appear")
	} else {
		if nm.Heavy == nil || !*nm.Heavy {
			t.Errorf("node_modules should have heavy: true, got %+v", nm)
		}
		if nm.Kind != apispec.FsEntryKindDir {
			t.Errorf("node_modules should have kind dir, got %v", nm.Kind)
		}
	}

	// 3. Out-of-scope symlink must be refused
	if outLink, ok := names["link_outside.txt"]; !ok {
		t.Error("link_outside.txt should appear")
	} else {
		if outLink.SymlinkRefused == nil || !*outLink.SymlinkRefused {
			t.Errorf("link_outside.txt should be refused, got %+v", outLink)
		}
	}

	// 4. In-scope symlink is accepted
	if inLink, ok := names["link_readme.md"]; !ok {
		t.Error("link_readme.md should appear")
	} else {
		if inLink.SymlinkRefused != nil && *inLink.SymlinkRefused {
			t.Errorf("link_readme.md should not be refused")
		}
		if inLink.SymlinkTarget == nil {
			t.Errorf("link_readme.md should have target set")
		}
	}

	// 5. Sorting: dirs first, then names case-insensitive
	entries := *res.Entries
	foundFile := false
	for _, e := range entries {
		if e.Kind == apispec.FsEntryKindDir {
			if foundFile {
				t.Errorf("dir %s appeared after a file in sorted order", e.Name)
			}
		} else {
			foundFile = true
		}
	}
}

func TestFsBrowse_500Cap(t *testing.T) {
	root := t.TempDir()
	ag, _ := newTestAgent(t, false, []string{root}, nil)

	// Create 520 files
	for i := 0; i < 520; i++ {
		name := filepath.Join(root, "file_"+strings.Repeat("0", 4-len(string(rune(i))))+string(rune(i))+".txt")
		_ = os.WriteFile(name, []byte("ok"), 0o644)
	}

	res, err := ag.ListDirectory(filepath.Base(root), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated == nil || !*res.Truncated {
		t.Fatal("expected truncated: true for > 500 files")
	}
	if len(*res.Entries) != 500 {
		t.Fatalf("expected exactly 500 entries, got: %d", len(*res.Entries))
	}
}

func TestFsBrowse_Search(t *testing.T) {
	root := t.TempDir()
	ag, _ := newTestAgent(t, false, []string{root}, nil)

	_ = os.MkdirAll(filepath.Join(root, "sub", "deep"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sub", "deep", "TargetFile.go"), []byte("code"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "sub", "ignore.txt"), []byte("code"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "TargetSpecial.js"), []byte("code"), 0o644)

	// Hidden and heavy dirs must be skipped
	_ = os.MkdirAll(filepath.Join(root, "node_modules", "target"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "node_modules", "target", "TargetModule.js"), []byte("code"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".git", "target"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".git", "target", "TargetGit.txt"), []byte("code"), 0o644)

	// Case-insensitive substring search
	res, err := ag.SearchFiles("target")
	if err != nil {
		t.Fatal(err)
	}
	if res.Results == nil || len(*res.Results) != 2 {
		t.Fatalf("expected 2 search results, got: %+v", res.Results)
	}

	// Glob search
	resGlob, err := ag.SearchFiles("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if resGlob.Results == nil || len(*resGlob.Results) != 1 || (*resGlob.Results)[0].Path != "sub/deep/TargetFile.go" {
		t.Fatalf("expected TargetFile.go for *.go glob, got: %+v", resGlob.Results)
	}
}

func TestFsBrowse_GitStatus(t *testing.T) {
	ctx := context.Background()
	repoDir := initTestRepo(t)
	ag, _ := newTestAgent(t, false, []string{repoDir}, nil)

	// 1. Modified file
	_ = os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("modified content"), 0o644)

	// 2. Added file (staged)
	_ = os.WriteFile(filepath.Join(repoDir, "staged.txt"), []byte("staged"), 0o644)
	_, _ = gitExec(ctx, repoDir, "add", "staged.txt")

	// 3. Untracked file
	_ = os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("new"), 0o644)

	// 4. File with spaces and newline in name (tests -z parsing)
	specialName := "special file with spaces and\nnewline.txt"
	_ = os.WriteFile(filepath.Join(repoDir, specialName), []byte("special"), 0o644)

	res, err := ag.GitStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.GitStatus == nil || len(*res.GitStatus) != 1 {
		t.Fatalf("expected 1 git status entry, got: %+v", res.GitStatus)
	}

	st := (*res.GitStatus)[0]
	if st.Branch == nil || *st.Branch == "" {
		t.Fatal("expected branch name to be populated")
	}

	filesMap := make(map[string]string)
	for _, f := range *st.Files {
		filesMap[f.Path] = f.Status
	}

	if _, ok := filesMap["README.md"]; !ok {
		t.Error("README.md should be reported in git status")
	}
	if _, ok := filesMap["staged.txt"]; !ok {
		t.Error("staged.txt should be reported in git status")
	}
	if _, ok := filesMap["untracked.txt"]; !ok {
		t.Error("untracked.txt should be reported in git status")
	}
	if _, ok := filesMap[specialName]; !ok {
		t.Errorf("special file with spaces and newline should be parsed correctly, files: %+v", filesMap)
	}
}

func TestFsBrowse_GitAttributesFilterNotRun(t *testing.T) {
	ctx := context.Background()
	repoDir := initTestRepo(t)
	ag, _ := newTestAgent(t, false, []string{repoDir}, nil)

	marker := filepath.Join(t.TempDir(), "filter-ran")
	cmd := exec.Command("git", "config", "filter.evil.clean", "touch "+marker+"; cat")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	_ = os.WriteFile(filepath.Join(repoDir, ".gitattributes"), []byte("* filter=evil\n"), 0o644)
	_ = os.WriteFile(filepath.Join(repoDir, "newfile.txt"), []byte("payload"), 0o644)

	_, err := ag.GitStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("evil filter ran during GitStatus!")
	}
}
