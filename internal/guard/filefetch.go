package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var secretPatterns = []string{
	".env", ".env.*", "*.pem", "*.key", "id_rsa*", "id_ed25519*", "*.p12", "credentials*", "*.token", "*.secret", ".netrc", ".git-credentials",
}

func isSecretName(base string) bool {
	for _, pat := range secretPatterns {
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
	}
	return false
}

func isDeniedPath(p, home string, denyFiles []string, exchangeDir string) bool {
	if home != "" {
		for _, d := range deniedPaths(home) {
			if within(p, d) {
				return true
			}
		}
		if within(p, filepath.Join(home, ".config/colonyhall")) {
			return true
		}
	}
	for _, df := range denyFiles {
		if df == "" {
			continue
		}
		cdf := filepath.Clean(df)
		if within(p, cdf) || p == cdf {
			return true
		}
		if rdf, err := filepath.EvalSymlinks(cdf); err == nil {
			if within(p, rdf) || p == rdf {
				return true
			}
		}
	}
	if exchangeDir != "" {
		cex := filepath.Clean(exchangeDir)
		if within(p, cex) || p == cex {
			return true
		}
		if rex, err := filepath.EvalSymlinks(cex); err == nil {
			if within(p, rex) || p == rex {
				return true
			}
		}
	}
	return false
}

// MayAccessPath checks whether a real (already resolved/cleaned) path is within the allowed scope
// and does not violate secret patterns or deny list rules.
func MayAccessPath(
	realPath string,
	roots []string,
	grants []Grant,
	sandboxed bool,
	home string,
	denyFiles []string,
	exchangeDir string,
) (bool, string) {
	if realPath == "" {
		return false, "denied"
	}
	realPath = filepath.Clean(realPath)

	if isSecretName(filepath.Base(realPath)) {
		return false, "denied"
	}
	if isDeniedPath(realPath, home, denyFiles, exchangeDir) {
		return false, "denied"
	}

	if sandboxed {
		insideGrant := false
		for _, g := range grants {
			if realGrant, reason := CheckGrant(g.Path, roots, home); reason == "" && realGrant != "" {
				if within(realPath, realGrant) {
					insideGrant = true
					break
				}
			}
		}
		if !insideGrant {
			return false, "denied"
		}
	} else {
		insideRoot := false
		for _, r := range roots {
			if within(realPath, r) {
				insideRoot = true
				break
			}
		}
		if !insideRoot {
			return false, "denied"
		}
	}
	return true, ""
}

// CheckFileFetch validates the requested path against scope, deny rules, symlinks, file type, and mime.
// Returns (realPath, mimeType, isImage, errorCode).
// If errorCode != "", the request must be failed with that errorCode.
func CheckFileFetch(
	rawPath string,
	roots []string,
	grants []Grant,
	sandboxed bool,
	home string,
	denyFiles []string,
	exchangeDir string,
) (string, string, bool, string) {
	if rawPath == "" {
		return "", "", false, "denied"
	}
	path := rawPath
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(path) {
		return "", "", false, "denied"
	}
	path = filepath.Clean(path)

	// Check secret patterns before symlink eval
	if isSecretName(filepath.Base(path)) {
		return "", "", false, "denied"
	}

	// Check deny list before symlink eval
	if isDeniedPath(path, home, denyFiles, exchangeDir) {
		return "", "", false, "denied"
	}

	// Symlink resolution
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return "", "", false, "not_found"
		}
		return "", "", false, "denied"
	}
	realPath = filepath.Clean(realPath)

	// Re-check secret patterns, deny list, and scope on resolved path
	if ok, code := MayAccessPath(realPath, roots, grants, sandboxed, home, denyFiles, exchangeDir); !ok {
		return "", "", false, code
	}

	// Regular file check
	fi, err := os.Stat(realPath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return "", "", false, "not_found"
		}
		return "", "", false, "denied"
	}
	if fi.IsDir() || !fi.Mode().IsRegular() {
		return "", "", false, "not_a_file"
	}

	// MIME detection
	mimeType, isImage := detectMIME(realPath)
	return realPath, mimeType, isImage, ""
}

func detectMIME(path string) (string, bool) {
	ext := filepath.Ext(path)
	extMIME := mime.TypeByExtension(ext)
	isImage := strings.HasPrefix(extMIME, "image/")

	f, err := os.Open(path)
	if err != nil {
		if extMIME != "" {
			return extMIME, isImage
		}
		return "application/octet-stream", false
	}
	defer func() { _ = f.Close() }()

	var buf [512]byte
	n, _ := io.ReadFull(f, buf[:])
	sniffedMIME := http.DetectContentType(buf[:n])
	if strings.HasPrefix(sniffedMIME, "image/") {
		isImage = true
	}

	chosenMIME := sniffedMIME
	if extMIME != "" && !strings.HasPrefix(extMIME, "text/plain") {
		chosenMIME = extMIME
	}
	return chosenMIME, isImage
}

// openSnapshot opens the checked path and makes sure the opened file is
// still that regular file: between the check and the open a writable folder
// could have swapped it for a symlink pointing elsewhere. The size is the
// one at open time, so a growing log is sent as a consistent snapshot.
func openSnapshot(realPath string) (*os.File, int64, error) {
	f, err := os.Open(realPath)
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, 0, errors.New("not a regular file")
	}
	if opened, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", f.Fd())); err == nil && opened != realPath {
		_ = f.Close()
		return nil, 0, fmt.Errorf("path changed after the check: %s", opened)
	}
	return f, fi.Size(), nil
}

func (a *Agent) homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

func (a *Agent) activeGrants() []Grant {
	if a.GrantsFunc != nil {
		return a.GrantsFunc()
	}
	if a.Policy != nil {
		return a.Policy().Grants
	}
	if a.Cfg.Dir != "" {
		return []Grant{{Path: a.Cfg.Dir, Mode: "write", Write: true}}
	}
	return nil
}

func (a *Agent) activeRoots() []string {
	if len(a.Roots) > 0 {
		return a.Roots
	}
	if a.RootDir != "" {
		return []string{a.RootDir}
	}
	if a.Cfg.RootDir != "" {
		return []string{a.Cfg.RootDir}
	}
	if a.Cfg.Dir != "" {
		return []string{a.Cfg.Dir}
	}
	return nil
}

func (a *Agent) fileFetchLimit() chan struct{} {
	if a.FileFetchLimiter != nil {
		return a.FileFetchLimiter
	}
	a.fileFetchMu.Lock()
	defer a.fileFetchMu.Unlock()
	if a.FileFetchLimiter == nil {
		a.FileFetchLimiter = make(chan struct{}, 2)
	}
	return a.FileFetchLimiter
}

func (a *Agent) runFileFetchLoop(ctx context.Context, wakeCh <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-wakeCh:
			a.pollAndServeFileFetches(ctx)
		}
	}
}

func (a *Agent) pollAndServeFileFetches(ctx context.Context) {
	if a.Client == nil {
		return
	}
	pending, err := a.Client.PendingFileFetches(ctx)
	if err != nil {
		if ctx.Err() == nil {
			a.Log.Debug("guard: check pending file fetches", "err", err)
		}
		return
	}
	if len(pending) == 0 {
		return
	}
	for _, req := range pending {
		a.fileFetchMu.Lock()
		if a.fileFetchInFlight == nil {
			a.fileFetchInFlight = make(map[string]bool)
		}
		if a.fileFetchInFlight[req.ID] {
			a.fileFetchMu.Unlock()
			continue
		}
		a.fileFetchInFlight[req.ID] = true
		a.fileFetchMu.Unlock()

		go func(r AgentPendingFileFetch) {
			defer func() {
				a.fileFetchMu.Lock()
				delete(a.fileFetchInFlight, r.ID)
				a.fileFetchMu.Unlock()
			}()
			limiter := a.fileFetchLimit()
			select {
			case limiter <- struct{}{}:
				defer func() { <-limiter }()
				a.serveFileFetch(ctx, r)
			case <-ctx.Done():
			}
		}(req)
	}
}

func (a *Agent) serveFileFetch(ctx context.Context, req AgentPendingFileFetch) {
	home := a.homeDir()
	realPath, mimeType, isImage, errCode := CheckFileFetch(
		req.Path,
		a.activeRoots(),
		a.activeGrants(),
		a.Sandbox,
		home,
		a.DenyFiles,
		a.ExchangeDir,
	)
	if errCode != "" {
		a.Log.Warn("guard: file fetch rejected", "agent", a.logName(), "path", req.Path, "error_code", errCode, "req_id", req.ID)
		if err := a.Client.FailFileFetch(ctx, req.ID, errCode); err != nil && ctx.Err() == nil {
			a.Log.Warn("guard: fail file fetch error", "err", err, "req_id", req.ID)
		}
		return
	}

	f, size, err := openSnapshot(realPath)
	if err != nil {
		code := "denied"
		if os.IsNotExist(err) {
			code = "not_found"
		}
		a.Log.Warn("guard: file fetch open refused", "agent", a.logName(), "path", req.Path, "err", err, "req_id", req.ID)
		_ = a.Client.FailFileFetch(ctx, req.ID, code)
		return
	}
	defer func() { _ = f.Close() }()

	limit := req.MaxFileBytes
	if isImage {
		limit = req.MaxImageBytes
	}
	if limit > 0 && size > limit {
		a.Log.Warn("guard: file fetch too large", "agent", a.logName(), "path", req.Path, "size", size, "limit", limit, "req_id", req.ID)
		_ = a.Client.FailFileFetch(ctx, req.ID, "too_large")
		return
	}
	fi, err := f.Stat()
	if err != nil {
		_ = a.Client.FailFileFetch(ctx, req.ID, "denied")
		return
	}

	filename := filepath.Base(realPath)
	mtime := fi.ModTime().UTC().Format(time.RFC3339)

	// A log grows while it is sent: send the size it had when opened.
	err = a.Client.UploadFetchedContent(ctx, req.ID, mimeType, filename, mtime, io.LimitReader(f, size), size)
	if err != nil {
		a.Log.Warn("guard: file fetch upload error", "agent", a.logName(), "path", req.Path, "err", err, "req_id", req.ID)
		return
	}

	a.Log.Info("guard: file fetch completed", "agent", a.logName(), "path", req.Path, "size", size, "req_id", req.ID)
}
