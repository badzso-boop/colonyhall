package guard

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
)

// Host-level folder listing (round 33). An admin's folder picker lists the
// folders of a machine before any agent exists there. The request reaches
// the guard over its own WebSocket (/ws/host, authenticated with the host
// token); the guard fetches it and posts the answer with the host token.
// The guard answers only inside its guard.yaml roots and never lists files.

// maxHostDirEntries bounds one listing (the server refuses more).
const maxHostDirEntries = 500

// hasChildrenScan bounds how many entries are read to decide has_children;
// a folder with more and no subfolder among them is reported as having
// children (the picker then shows an empty folder, never a missing one).
const hasChildrenScan = 2000

// hostDeniedNames are folder names (or name paths) that are secret wherever
// they appear under a root, not only in the guard user's home.
var hostDeniedNames = append(slices.Clone(deniedHome), ".config/colonyhall")

// HostFsScope is what a host listing may see: the resolved roots and the
// deny rules of this machine.
type HostFsScope struct {
	Roots       []string // resolved (ResolveRoots)
	Home        string
	DenyFiles   []string
	ExchangeDir string
}

func listingErr(path string, code apispec.HostDirListingErrorCode) apispec.HostDirListing {
	return apispec.HostDirListing{Op: apispec.HostDirListingOpListDirs, Path: &path, Entries: []apispec.HostDirEntry{}, ErrorCode: &code}
}

// secretDir reports whether p is, or lies inside, a secret-looking folder:
// the guard's deny list by name anywhere (.ssh, .gnupg, .aws,
// .config/colonyhall, …), secret file patterns, the token files and the
// secrets exchange folder.
func (sc HostFsScope) secretDir(p string) bool {
	slashed := filepath.ToSlash(p) + "/"
	for _, d := range hostDeniedNames {
		if strings.Contains(slashed, "/"+d+"/") {
			return true
		}
	}
	if isSecretName(filepath.Base(p)) {
		return true
	}
	return isDeniedPath(p, sc.Home, sc.DenyFiles, sc.ExchangeDir)
}

func (sc HostFsScope) inRoots(real string) bool {
	for _, r := range sc.Roots {
		if within(real, r) {
			return true
		}
	}
	return false
}

// ListDirs lists the folders inside path, or the roots when path is empty.
func (sc HostFsScope) ListDirs(path string) apispec.HostDirListing {
	if path == "" {
		entries := make([]apispec.HostDirEntry, 0, len(sc.Roots))
		for _, r := range sc.Roots {
			e := sc.entry(r, r, r)
			e.Name = r
			entries = append(entries, e)
		}
		empty := ""
		f := false
		return apispec.HostDirListing{Op: apispec.HostDirListingOpListDirs, Path: &empty, Entries: entries, Truncated: &f}
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return listingErr(path, apispec.HostDirListingErrorCodeDenied)
	}
	if sc.secretDir(path) {
		return listingErr(path, apispec.HostDirListingErrorCodeDenied)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Do not tell what exists outside the roots.
			if !sc.inRoots(path) {
				return listingErr(path, apispec.HostDirListingErrorCodeDenied)
			}
			return listingErr(path, apispec.HostDirListingErrorCodeNotFound)
		}
		return listingErr(path, apispec.HostDirListingErrorCodeDenied)
	}
	if !sc.inRoots(real) || sc.secretDir(real) {
		return listingErr(path, apispec.HostDirListingErrorCodeDenied)
	}
	fi, err := os.Stat(real)
	if err != nil {
		return listingErr(path, apispec.HostDirListingErrorCodeNotFound)
	}
	if !fi.IsDir() {
		return listingErr(path, apispec.HostDirListingErrorCodeNotADirectory)
	}
	des, err := os.ReadDir(real)
	if err != nil {
		return listingErr(path, apispec.HostDirListingErrorCodeDenied)
	}

	entries := make([]apispec.HostDirEntry, 0)
	for _, de := range des {
		name := de.Name()
		shown := filepath.Join(path, name)
		target := filepath.Join(real, name)
		if de.Type()&os.ModeSymlink != 0 {
			// Follow a symlink only when it stays inside the roots.
			resolved, err := filepath.EvalSymlinks(target)
			if err != nil || !sc.inRoots(resolved) {
				continue
			}
			if st, err := os.Stat(resolved); err != nil || !st.IsDir() {
				continue
			}
			target = resolved
		} else if !de.IsDir() {
			continue
		}
		entries = append(entries, sc.entry(name, shown, target))
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a != b {
			return a < b
		}
		return entries[i].Name < entries[j].Name
	})
	truncated := len(entries) > maxHostDirEntries
	if truncated {
		entries = entries[:maxHostDirEntries]
	}
	return apispec.HostDirListing{Op: apispec.HostDirListingOpListDirs, Path: &path, Entries: entries, Truncated: &truncated}
}

// entry describes one folder; shown is the path the picker uses, real the
// resolved folder that is inspected.
func (sc HostFsScope) entry(name, shown, real string) apispec.HostDirEntry {
	base := filepath.Base(shown)
	hidden := strings.HasPrefix(base, ".")
	heavy := base == ".git" || heavyDirs[base]
	e := apispec.HostDirEntry{Name: name, Path: shown, Hidden: &hidden, Heavy: &heavy}
	if sc.secretDir(shown) || sc.secretDir(real) {
		denied := true
		e.Denied = &denied
		return e
	}
	isGit := false
	if _, err := os.Lstat(filepath.Join(real, ".git")); err == nil {
		isGit = true
	}
	children := sc.hasChildren(real)
	e.IsGitRepo = &isGit
	e.HasChildren = &children
	return e
}

func (sc HostFsScope) hasChildren(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	scanned := 0
	for scanned < hasChildrenScan {
		des, err := f.ReadDir(256)
		for _, de := range des {
			if de.IsDir() {
				return true
			}
			if de.Type()&os.ModeSymlink != 0 {
				if resolved, err := filepath.EvalSymlinks(filepath.Join(dir, de.Name())); err == nil && sc.inRoots(resolved) {
					if st, err := os.Stat(resolved); err == nil && st.IsDir() {
						return true
					}
				}
			}
		}
		scanned += len(des)
		if err != nil {
			return false // io.EOF or a read error: no subfolder seen
		}
	}
	return true
}

// GetHostFsRequest fetches a pending host fs request (host token).
func (c *Client) GetHostFsRequest(ctx context.Context, requestID string) (apispec.HostFsPendingRequest, error) {
	var out apispec.HostFsPendingRequest
	err := c.do(ctx, http.MethodGet, "/api/v1/host/fs/"+url.PathEscape(requestID), nil, &out)
	return out, err
}

// PostHostFsResult delivers a host fs answer (host token).
func (c *Client) PostHostFsResult(ctx context.Context, requestID string, res apispec.HostDirListing) error {
	return c.do(ctx, http.MethodPost, "/api/v1/host/fs/"+url.PathEscape(requestID)+"/result", res, nil)
}

// ServeHostFsRequest fetches one request, answers it within scope and
// posts the result.
func ServeHostFsRequest(ctx context.Context, c *Client, scope HostFsScope, requestID string, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := c.GetHostFsRequest(ctx, requestID)
	if err != nil {
		log.Warn("guard: get host fs request failed", "req_id", requestID, "err", err)
		return
	}
	var res apispec.HostDirListing
	switch req.Op {
	case apispec.HostFsPendingRequestOpListDirs:
		res = scope.ListDirs(deref(req.Path))
	default:
		res = listingErr(deref(req.Path), apispec.HostDirListingErrorCodeDenied)
	}
	if err := c.PostHostFsResult(ctx, requestID, res); err != nil {
		log.Warn("guard: post host fs result failed", "req_id", requestID, "err", err)
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// HostWS holds the guard's host WebSocket (/ws/host): it authenticates with
// the host token and serves host.fs.request frames. It reconnects with
// backoff until ctx ends.
type HostWS struct {
	Client  *Client // host token
	Scope   HostFsScope
	Log     *slog.Logger
	Backoff func(attempt int) time.Duration // wsBackoff when nil
}

// Run keeps the connection open until ctx ends.
func (h *HostWS) Run(ctx context.Context) {
	backoff := h.Backoff
	if backoff == nil {
		backoff = wsBackoff
	}
	attempt := 0
	for ctx.Err() == nil {
		connected, err := h.runConn(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			attempt = 0
		}
		d := backoff(attempt)
		attempt++
		h.Log.Debug("guard: host websocket reconnecting", "backoff", d, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

func (h *HostWS) runConn(ctx context.Context) (bool, error) {
	u, err := wsURLPath(h.Client.BaseURL, "/ws/host")
	if err != nil {
		return false, err
	}
	// The dial client must not carry a Timeout (it would end the socket).
	opts := &websocket.DialOptions{}
	if h.Client.HTTP != nil {
		opts.HTTPClient = &http.Client{Transport: h.Client.HTTP.Transport}
	}
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, resp, err := websocket.Dial(connCtx, u, opts)
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.CloseNow() }()
	token, err := h.Client.CurrentToken(connCtx)
	if err != nil {
		return false, err
	}
	helloCtx, helloCancel := context.WithTimeout(connCtx, 10*time.Second)
	err = wsjson.Write(helloCtx, conn, map[string]any{"type": "hello", "token": token})
	helloCancel()
	if err != nil {
		return false, err
	}
	welcomed := false
	for {
		var frame struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
		}
		if err := wsjson.Read(connCtx, conn, &frame); err != nil {
			return welcomed, err
		}
		switch frame.Type {
		case "welcome":
			if !welcomed {
				h.Log.Debug("guard: host websocket connected")
			}
			welcomed = true
		case "host.fs.request":
			if frame.RequestID != "" {
				go ServeHostFsRequest(connCtx, h.Client, h.Scope, frame.RequestID, h.Log)
			}
		}
	}
}
