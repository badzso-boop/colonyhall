// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"colonyhall/internal/apispec"
)

// ClaudeLogin is the guard's Claude Code login token (`claude setup-token`,
// round 38): a person sets it from the PWA for a machine they own, and the
// guard gives it to every Claude Code run as CLAUDE_CODE_OAUTH_TOKEN, in the
// child's environment only. The file is 0600 in the guard's config folder
// and on every agent's deny list. The token never goes to a log, the
// server's database or a report: the report carries only when it was set.
type ClaudeLogin struct {
	Path string // claude_token_file; "" disables setting it from the PWA

	mu    sync.RWMutex
	token string
	setAt *time.Time
}

// LoadClaudeLogin reads the token file; a missing file is no token.
func LoadClaudeLogin(path string) (*ClaudeLogin, error) {
	l := &ClaudeLogin{Path: path}
	if path == "" {
		return l, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("guard: claude_token_file: %w", err)
	}
	l.token = strings.TrimSpace(string(raw))
	if st, err := os.Stat(path); err == nil && l.token != "" {
		t := st.ModTime().UTC()
		l.setAt = &t
	}
	return l, nil
}

// Token is the current token ("" when none). Safe on a nil receiver.
func (l *ClaudeLogin) Token() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.token
}

// SetAt is when the token file was written (nil when there is none).
func (l *ClaudeLogin) SetAt() *time.Time {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.setAt
}

// Save writes the token atomically with mode 0600 and starts using it.
func (l *ClaudeLogin) Save(token string, now time.Time) error {
	if l.Path == "" {
		return errLoginNotConfigured
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.Path), ".claude-token-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(token + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), l.Path); err != nil {
		return err
	}
	t := now.UTC()
	l.mu.Lock()
	l.token, l.setAt = token, &t
	l.mu.Unlock()
	return nil
}

// Clear removes the token file and stops using the token.
func (l *ClaudeLogin) Clear() error {
	if l.Path == "" {
		return errLoginNotConfigured
	}
	if err := os.Remove(l.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	l.mu.Lock()
	l.token, l.setAt = "", nil
	l.mu.Unlock()
	return nil
}

var errLoginNotConfigured = errors.New("claude_token_file is not set in guard.yaml")

// LoginChecker runs `claude -p /usage` with a candidate token (no model
// call) and reports whether Claude Code accepted it.
type LoginChecker func(ctx context.Context, token string) bool

// NewLoginChecker checks a token with the Claude Code binary bin, in dir
// (an empty folder of the guard's own), with the token in the environment.
func NewLoginChecker(run Runner, bin, dir string) LoginChecker {
	return func(ctx context.Context, token string) bool {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return false
		}
		cctx, cancel := context.WithTimeout(ctx, usageTimeout)
		defer cancel()
		env := append(os.Environ(), "CLAUDE_CODE_OAUTH_TOKEN="+token)
		stdout, _, err := run(cctx, dir, bin, claudeUsageArgs, env)
		if err != nil {
			return false
		}
		_, ok := parseClaudeUsage(stdout, time.Now())
		return ok
	}
}

// GetClaudeLoginRequest fetches a pending login request (host token). The
// answer carries the token once; the server forgets it on this read.
func (c *Client) GetClaudeLoginRequest(ctx context.Context, requestID string) (apispec.HostClaudeLoginRequest, error) {
	var out apispec.HostClaudeLoginRequest
	err := c.do(ctx, http.MethodGet, "/api/v1/host/claude-login/"+url.PathEscape(requestID), nil, &out)
	return out, err
}

// PostClaudeLoginResult answers a login request (host token).
func (c *Client) PostClaudeLoginResult(ctx context.Context, requestID string, res apispec.HostClaudeLoginResult) error {
	return c.do(ctx, http.MethodPost, "/api/v1/host/claude-login/"+url.PathEscape(requestID)+"/result", res, nil)
}

// ServeClaudeLoginRequest fetches one request, sets or clears the token
// and answers. A new token is checked first; a failing one keeps the old.
// onChange runs after a change (the guard reports the new state).
func ServeClaudeLoginRequest(ctx context.Context, c *Client, login *ClaudeLogin, check LoginChecker, requestID string, log *slog.Logger, onChange func()) {
	ctx, cancel := context.WithTimeout(ctx, usageTimeout+15*time.Second)
	defer cancel()
	req, err := c.GetClaudeLoginRequest(ctx, requestID)
	if err != nil {
		log.Warn("guard: get claude login request failed", "req_id", requestID, "err", err)
		return
	}
	res := serveClaudeLogin(ctx, login, check, req)
	if res.Ok && onChange != nil {
		onChange()
	}
	if res.ErrorCode != nil {
		log.Warn("guard: claude login not changed", "req_id", requestID, "op", req.Op, "reason", *res.ErrorCode)
	} else {
		log.Info("guard: claude login changed", "req_id", requestID, "op", req.Op)
	}
	if err := c.PostClaudeLoginResult(ctx, requestID, res); err != nil {
		log.Warn("guard: post claude login result failed", "req_id", requestID, "err", err)
	}
}

func serveClaudeLogin(ctx context.Context, login *ClaudeLogin, check LoginChecker, req apispec.HostClaudeLoginRequest) apispec.HostClaudeLoginResult {
	fail := func(code apispec.HostClaudeLoginResultErrorCode) apispec.HostClaudeLoginResult {
		return apispec.HostClaudeLoginResult{Ok: false, ErrorCode: &code}
	}
	if login == nil || login.Path == "" {
		return fail(apispec.NotConfigured)
	}
	switch req.Op {
	case apispec.Clear:
		if err := login.Clear(); err != nil {
			return fail(apispec.WriteFailed)
		}
	case apispec.Set:
		token := strings.TrimSpace(deref(req.Token))
		if token == "" || strings.ContainsAny(token, " \t\r\n") {
			return fail(apispec.CheckFailed)
		}
		if check == nil || !check(ctx, token) {
			return fail(apispec.CheckFailed)
		}
		if err := login.Save(token, time.Now()); err != nil {
			return fail(apispec.WriteFailed)
		}
	default:
		return fail(apispec.CheckFailed)
	}
	r := apispec.HostClaudeLoginResult{Ok: true}
	r.SetAt = login.SetAt()
	return r
}
