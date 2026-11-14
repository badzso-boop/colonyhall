// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

var heavyDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
}

// ScopeRoot describes a directory within the agent's visible filesystem scope.
type ScopeRoot struct {
	Name string
	Path string
	Mode string
}

// ScopeRoots returns the top-level scope roots for the agent.
// For full-access agents: root folder or worktree.
// For sandboxed agents: active grants and worktree.
func (a *Agent) ScopeRoots() []ScopeRoot {
	var out []ScopeRoot
	seen := make(map[string]bool)

	add := func(name, path, mode string) {
		path = filepath.Clean(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		if name == "" {
			name = filepath.Base(path)
		}
		out = append(out, ScopeRoot{Name: name, Path: path, Mode: mode})
	}

	// 1. Worktree
	if a.WorktreePath != "" {
		add("worktree", a.WorktreePath, "write")
	} else if a.WorktreeRepo != "" {
		handle := a.getMe().Handle
		if handle == "" {
			handle = a.Cfg.Name
		}
		if handle != "" {
			cleanRepo := filepath.Clean(a.WorktreeRepo)
			wtDir := cleanRepo + "-worktrees"
			wtPath := filepath.Join(wtDir, handle)
			if fi, err := os.Stat(wtPath); err == nil && fi.IsDir() {
				add("worktree", wtPath, "write")
			}
		}
	}

	// 2. Roots / Grants
	if a.Sandbox {
		for _, g := range a.activeGrants() {
			mode := "read"
			if g.Write || g.Mode == "write" {
				mode = "write"
			}
			add(filepath.Base(g.Path), g.Path, mode)
		}
	} else if a.RootDir != "" {
		// A full-access agent's browser starts where the agent works: its
		// root folder, not every root of the machine (which would show only
		// the project folders, e.g. /home/alice/projects).
		add(filepath.Base(a.RootDir), a.RootDir, "write")
	} else {
		for _, r := range a.activeRoots() {
			add(filepath.Base(r), r, "write")
		}
	}

	return out
}

func (a *Agent) findScopeRoot(roots []ScopeRoot, rootArg string) (ScopeRoot, bool) {
	if len(roots) == 0 {
		return ScopeRoot{}, false
	}
	if rootArg == "" {
		return roots[0], true
	}
	cleanArg := filepath.Clean(rootArg)
	for _, r := range roots {
		if r.Name == rootArg || r.Path == cleanArg || filepath.Base(r.Path) == rootArg {
			return r, true
		}
	}
	return ScopeRoot{}, false
}

func (a *Agent) isHidden(name, fullPath, home string) bool {
	if name == ".git" {
		return true
	}
	if isSecretName(name) {
		return true
	}
	if isDeniedPath(fullPath, home, a.DenyFiles, a.ExchangeDir) {
		return true
	}
	return false
}

func opPtr(op apispec.AgentFsResponseOp) *apispec.AgentFsResponseOp {
	return &op
}

func strPtr(s string) *string {
	return &s
}

// ListDirectory lists a single level of entries within the selected scope root and subpath.
func (a *Agent) ListDirectory(rootArg, pathArg string) (*apispec.AgentFsResponse, error) {
	home := a.homeDir()
	scopeRoots := a.ScopeRoots()
	if len(scopeRoots) == 0 {
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: strPtr("denied")}, nil
	}

	var apiRoots []apispec.FsRoot
	for _, r := range scopeRoots {
		m := r.Mode
		apiRoots = append(apiRoots, apispec.FsRoot{
			Name: r.Name,
			Path: r.Path,
			Mode: &m,
		})
	}

	// If there are multiple roots and neither root nor path is specified, return roots.
	if len(scopeRoots) > 1 && rootArg == "" && pathArg == "" {
		return &apispec.AgentFsResponse{
			Op:    opPtr(apispec.AgentFsResponseOpList),
			Roots: &apiRoots,
		}, nil
	}

	root, ok := a.findScopeRoot(scopeRoots, rootArg)
	if !ok {
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: strPtr("denied")}, nil
	}

	cleanRel := filepath.Clean(filepath.ToSlash(pathArg))
	if cleanRel == "." || cleanRel == "/" {
		cleanRel = ""
	}
	if filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, "..") {
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: strPtr("denied")}, nil
	}

	targetDir := root.Path
	if cleanRel != "" {
		targetDir = filepath.Join(root.Path, cleanRel)
	}

	// Verify the real folder is accessible: a symlink inside the scope
	// (x -> / or x -> ~/.ssh) must not list what is outside it (round 40).
	realDir, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		code := "denied"
		if os.IsNotExist(err) {
			code = "not_found"
		}
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: &code}, nil
	}
	if allowed, reason := MayAccessPath(realDir, a.activeRoots(), a.activeGrants(), a.Sandbox, home, a.DenyFiles, a.ExchangeDir); !allowed {
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: &reason}, nil
	}

	dirEntries, err := os.ReadDir(realDir)
	if err != nil {
		code := "denied"
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			code = "not_found"
		}
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: &code}, nil
	}

	var entries []apispec.FsEntry
	for _, de := range dirEntries {
		name := de.Name()
		fullPath := filepath.Join(targetDir, name)

		if a.isHidden(name, fullPath, home) {
			continue
		}

		isDir := de.IsDir()
		kind := apispec.FsEntryKindFile
		if isDir {
			kind = apispec.FsEntryKindDir
		}

		var heavy *bool
		if isDir && heavyDirs[name] {
			h := true
			heavy = &h
		}

		var symlinkTarget *string
		var symlinkRefused *bool

		if de.Type()&os.ModeSymlink != 0 {
			kind = apispec.FsEntryKindSymlink
			resolved, err := filepath.EvalSymlinks(fullPath)
			if err != nil {
				ref := true
				symlinkRefused = &ref
			} else {
				if allowed, _ := MayAccessPath(resolved, a.activeRoots(), a.activeGrants(), a.Sandbox, home, a.DenyFiles, a.ExchangeDir); !allowed {
					ref := true
					symlinkRefused = &ref
				} else {
					st := resolved
					symlinkTarget = &st
					if fi, err := os.Stat(resolved); err == nil && fi.IsDir() {
						kind = apispec.FsEntryKindDir
					} else {
						kind = apispec.FsEntryKindFile
					}
				}
			}
		}

		var size *int64
		var mtimeStr *string
		if fi, err := de.Info(); err == nil {
			s := fi.Size()
			size = &s
			mt := fi.ModTime().UTC().Format(time.RFC3339)
			mtimeStr = &mt
		}

		var mtimeVal *time.Time
		if mtimeStr != nil {
			if t, err := time.Parse(time.RFC3339, *mtimeStr); err == nil {
				mtimeVal = &t
			}
		}

		entries = append(entries, apispec.FsEntry{
			Name:           name,
			Kind:           kind,
			Size:           size,
			Mtime:          mtimeVal,
			Heavy:          heavy,
			SymlinkTarget:  symlinkTarget,
			SymlinkRefused: symlinkRefused,
		})
	}

	// Sort dirs first, then names case-insensitive
	sort.Slice(entries, func(i, j int) bool {
		iDir := entries[i].Kind == apispec.FsEntryKindDir
		jDir := entries[j].Kind == apispec.FsEntryKindDir
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	truncated := false
	if len(entries) > 500 {
		truncated = true
		entries = entries[:500]
	}

	return &apispec.AgentFsResponse{
		Op:        opPtr(apispec.AgentFsResponseOpList),
		Roots:     &apiRoots,
		Entries:   &entries,
		Truncated: &truncated,
	}, nil
}

// SearchFiles searches for matching file names within the agent's scope roots.
func (a *Agent) SearchFiles(query string) (*apispec.AgentFsResponse, error) {
	home := a.homeDir()
	scopeRoots := a.ScopeRoots()
	if len(scopeRoots) == 0 {
		return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpSearch), ErrorCode: strPtr("denied")}, nil
	}

	startTime := time.Now()
	timeoutDur := 3 * time.Second
	maxResults := 200

	qLower := strings.ToLower(query)
	isGlob := strings.Contains(query, "*")

	matches := func(name string) bool {
		nLower := strings.ToLower(name)
		if isGlob {
			matched, _ := filepath.Match(qLower, nLower)
			return matched
		}
		return strings.Contains(nLower, qLower)
	}

	var results []apispec.FsSearchResult
	truncated := false
	var reason string

	for _, root := range scopeRoots {
		if truncated {
			break
		}

		_ = filepath.WalkDir(root.Path, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if time.Since(startTime) >= timeoutDur {
				truncated = true
				reason = "timeout"
				return filepath.SkipAll
			}
			if len(results) >= maxResults {
				truncated = true
				reason = "limit"
				return filepath.SkipAll
			}

			name := d.Name()
			if a.isHidden(name, path, home) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				if path != root.Path && heavyDirs[name] {
					return filepath.SkipDir
				}
				return nil
			}

			if matches(name) {
				rel, err := filepath.Rel(root.Path, path)
				if err == nil {
					results = append(results, apispec.FsSearchResult{
						Root: root.Name,
						Path: filepath.ToSlash(rel),
					})
				}
			}
			return nil
		})
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	return &apispec.AgentFsResponse{
		Op:        opPtr(apispec.AgentFsResponseOpSearch),
		Results:   &results,
		Truncated: &truncated,
		Reason:    reasonPtr,
	}, nil
}

// GitStatus inspects git repositories for each scope root that is inside a git work tree.
func (a *Agent) GitStatus(ctx context.Context) (*apispec.AgentFsResponse, error) {
	scopeRoots := a.ScopeRoots()
	var statuses []apispec.FsGitStatus

	for _, root := range scopeRoots {
		tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := gitExec(tctx, root.Path, "rev-parse", "--is-inside-work-tree")
		cancel()
		if err != nil {
			continue // Not a git work tree
		}

		st := apispec.FsGitStatus{
			Root: root.Name,
		}

		tctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cmdPrefix := []string{"--attr-source=" + emptyTree}

		// 1. Branch
		branchOut, err := gitExec(tctx, root.Path, append(cmdPrefix, "symbolic-ref", "--short", "HEAD")...)
		if err != nil {
			headSha, err2 := gitExec(tctx, root.Path, append(cmdPrefix, "rev-parse", "--short", "HEAD")...)
			if err2 == nil {
				st.Branch = strPtr("detached (" + headSha + ")")
			} else if errors.Is(tctx.Err(), context.DeadlineExceeded) {
				st.ErrorCode = strPtr("timeout")
				statuses = append(statuses, st)
				continue
			} else {
				st.Branch = strPtr("unknown")
			}
		} else {
			st.Branch = strPtr(branchOut)
		}

		// 2. Ahead / Behind
		countOut, err := gitExec(tctx, root.Path, append(cmdPrefix, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")...)
		if err == nil {
			parts := strings.Fields(countOut)
			if len(parts) == 2 {
				behind, _ := strconv.Atoi(parts[0])
				ahead, _ := strconv.Atoi(parts[1])
				st.Ahead = &ahead
				st.Behind = &behind
			}
		}

		// 3. Status porcelain=v1 -z
		statusOut, err := gitExecRaw(tctx, root.Path, append(cmdPrefix, "status", "--porcelain=v1", "-z")...)
		if err != nil {
			if errors.Is(tctx.Err(), context.DeadlineExceeded) {
				st.ErrorCode = strPtr("timeout")
			}
		} else {
			var files []apispec.FsGitFile
			records := strings.Split(string(statusOut), "\x00")
			for i := 0; i < len(records); i++ {
				rec := records[i]
				if len(rec) < 3 {
					continue
				}
				xy := rec[:2]
				filePath := rec[3:]
				status := strings.TrimSpace(xy)

				// In porcelain=v1 -z, renames and copies have two consecutive records
				if strings.HasPrefix(xy, "R") || strings.HasPrefix(xy, "C") {
					if i+1 < len(records) {
						i++ // skip orig path
					}
				}

				files = append(files, apispec.FsGitFile{
					Path:   filepath.ToSlash(filePath),
					Status: status,
				})
			}
			st.Files = &files
		}

		statuses = append(statuses, st)
	}

	return &apispec.AgentFsResponse{
		Op:        opPtr(apispec.AgentFsResponseOpGitStatus),
		GitStatus: &statuses,
	}, nil
}

// ExecuteFsOperation executes an incoming pending filesystem request.
func (a *Agent) ExecuteFsOperation(ctx context.Context, pending *apispec.AgentFsPendingRequest) *apispec.AgentFsResponse {
	var root, path, query string
	if pending.Root != nil {
		root = *pending.Root
	}
	if pending.Path != nil {
		path = *pending.Path
	}
	if pending.Query != nil {
		query = *pending.Query
	}

	a.Log.Info("guard: fs op", "agent", a.logName(), "op", pending.Op, "path", path)

	switch pending.Op {
	case apispec.AgentFsPendingRequestOpList:
		res, err := a.ListDirectory(root, path)
		if err != nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpList), ErrorCode: strPtr("denied")}
		}
		return res
	case apispec.AgentFsPendingRequestOpSearch:
		res, err := a.SearchFiles(query)
		if err != nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpSearch), ErrorCode: strPtr("denied")}
		}
		return res
	case apispec.AgentFsPendingRequestOpGitStatus:
		res, err := a.GitStatus(ctx)
		if err != nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpGitStatus), ErrorCode: strPtr("denied")}
		}
		return res
	case apispec.AgentFsPendingRequestOpTermList:
		if a.Terminals == nil {
			empty := make([]apispec.TerminalSummary, 0)
			return &apispec.AgentFsResponse{
				Op:        opPtr(apispec.AgentFsResponseOpTermList),
				Terminals: &empty,
			}
		}
		handle := a.getMe().Handle
		if handle == "" {
			handle = a.Cfg.Name
		}
		list, err := a.Terminals.List(handle)
		if err != nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpTermList), ErrorCode: strPtr("error")}
		}
		now := time.Now()
		summaries := make([]apispec.TerminalSummary, 0, len(list))
		for _, m := range list {
			var chUUID openapi_types.UUID
			if u, err := uuid.Parse(m.ChannelID); err == nil {
				chUUID = openapi_types.UUID(u)
			}
			var trUUID *openapi_types.UUID
			if m.ThreadRootID != "" {
				if u, err := uuid.Parse(m.ThreadRootID); err == nil {
					parsed := openapi_types.UUID(u)
					trUUID = &parsed
				}
			}
			var finAt *time.Time
			if m.FinishedAt != nil {
				tCopy := *m.FinishedAt
				finAt = &tCopy
			}
			var killedBy *string
			if m.KilledBy != "" {
				k := m.KilledBy
				killedBy = &k
			}
			summaries = append(summaries, apispec.TerminalSummary{
				Name:         m.Name,
				Status:       apispec.TerminalSummaryStatus(m.Status()),
				Command:      m.Command,
				Cwd:          m.Cwd,
				ChannelId:    chUUID,
				ThreadRootId: trUUID,
				StartedAt:    m.StartedAt,
				FinishedAt:   finAt,
				ExitCode:     m.ExitCode,
				KilledBy:     killedBy,
				DurationMs:   m.Duration(now).Milliseconds(),
			})
		}
		return &apispec.AgentFsResponse{
			Op:        opPtr(apispec.AgentFsResponseOpTermList),
			Terminals: &summaries,
		}
	case apispec.AgentFsPendingRequestOpTermOutput:
		if a.Terminals == nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpTermOutput), ErrorCode: strPtr("not_found")}
		}
		name := ""
		if pending.Name != nil {
			name = *pending.Name
		}
		lines := 1000
		if pending.Lines != nil && *pending.Lines > 0 {
			lines = *pending.Lines
			if lines > 1000 {
				lines = 1000
			}
		}
		handle := a.getMe().Handle
		if handle == "" {
			handle = a.Cfg.Name
		}
		out, err := a.Terminals.Output(handle, name, lines)
		if err != nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpTermOutput), ErrorCode: strPtr("not_found")}
		}
		filtered := FilterTerminalOutput(out)
		return &apispec.AgentFsResponse{
			Op:     opPtr(apispec.AgentFsResponseOpTermOutput),
			Output: &filtered,
		}
	case apispec.AgentFsPendingRequestOpTermKill:
		if a.Terminals == nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpTermKill), ErrorCode: strPtr("not_found")}
		}
		name := ""
		if pending.Name != nil {
			name = *pending.Name
		}
		killedBy := ""
		if pending.KilledBy != nil {
			killedBy = *pending.KilledBy
		}
		handle := a.getMe().Handle
		if handle == "" {
			handle = a.Cfg.Name
		}
		err := a.Terminals.Kill(handle, name, killedBy)
		if err != nil {
			return &apispec.AgentFsResponse{Op: opPtr(apispec.AgentFsResponseOpTermKill), ErrorCode: strPtr("error")}
		}
		killed := true
		return &apispec.AgentFsResponse{
			Op:     opPtr(apispec.AgentFsResponseOpTermKill),
			Killed: &killed,
		}
	default:
		return &apispec.AgentFsResponse{ErrorCode: strPtr("denied")}
	}
}

func (a *Agent) serveFsRequest(ctx context.Context, requestID string) {
	if a.Client == nil {
		return
	}
	pending, err := a.Client.GetFsRequest(ctx, requestID)
	if err != nil {
		a.Log.Warn("guard: get fs request failed", "agent", a.logName(), "req_id", requestID, "err", err)
		return
	}

	res := a.ExecuteFsOperation(ctx, pending)
	if err := a.Client.PostFsResult(ctx, requestID, res); err != nil {
		a.Log.Warn("guard: post fs result failed", "agent", a.logName(), "req_id", requestID, "err", err)
	}
}
