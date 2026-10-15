package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the guard's REST client for one agent. The guard is an ordinary
// API client: it never reaches into the server's internals.
type Client struct {
	BaseURL string
	Token   string
	// Tokens, when set, supplies the token instead of Token (host mode:
	// short-lived agent tokens the guard requests and renews).
	Tokens TokenSource
	HTTP   *http.Client
}

// CurrentToken returns the agent's current bearer token.
func (c *Client) CurrentToken(ctx context.Context) (string, error) {
	if c.Tokens != nil {
		return c.Tokens.Token(ctx)
	}
	return c.Token, nil
}

// TokenSource hands out a valid token; Invalidate drops a rejected one.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
}

// Me is the agent as /me returns it.
type Me struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	State  string `json:"state"`
	Halted bool   `json:"halted"`
}

// Item is one inbox item.
type Item struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	ItemID       string    `json:"item_id"`
	Channel      string    `json:"channel"`
	ChannelID    string    `json:"channel_id"`
	ThreadRootID string    `json:"thread_root_id"`
	Author       string    `json:"author"`
	AuthorKind   string    `json:"author_kind"`
	Body         string    `json:"body"`
	Mentioned    bool      `json:"mentioned"`
	DM           bool      `json:"dm"`
	At           time.Time `json:"at"`
}

// Message is a chat message (only what the guard needs).
type Message struct {
	ID       string `json:"id"`
	AuthorID string `json:"author_id"`
	Body     string `json:"body"`
}

// APIError is a non-2xx answer.
type APIError struct {
	Status int
	Detail string
}

func (e *APIError) Error() string { return fmt.Sprintf("colonyhall: HTTP %d: %s", e.Status, e.Detail) }

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	_, err := c.doH(ctx, method, path, nil, in, out)
	return err
}

func (c *Client) doH(ctx context.Context, method, path string, hdr map[string]string, in, out any) (http.Header, error) {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.BaseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	token := c.Token
	if c.Tokens != nil {
		if token, err = c.Tokens.Token(ctx); err != nil {
			return nil, err
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized && c.Tokens != nil {
		c.Tokens.Invalidate()
	}
	if res.StatusCode >= 300 {
		var p struct{ Detail, Title string }
		_ = json.Unmarshal(raw, &p)
		if p.Detail == "" {
			p.Detail = p.Title
		}
		return res.Header, &APIError{Status: res.StatusCode, Detail: p.Detail}
	}
	if out != nil && len(raw) > 0 {
		return res.Header, json.Unmarshal(raw, out)
	}
	return res.Header, nil
}

// Me returns the agent with its current state.
func (c *Client) Me(ctx context.Context) (Me, error) {
	var m Me
	err := c.do(ctx, http.MethodGet, "/api/v1/me", nil, &m)
	return m, err
}

// Inbox returns up to limit pending items and the total pending.
func (c *Client) Inbox(ctx context.Context, limit int) ([]Item, int, error) {
	var r struct {
		Items []Item `json:"items"`
		Total int    `json:"total"`
	}
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/agent/inbox?limit=%d", limit), nil, &r)
	return r.Items, r.Total, err
}

// Ack marks inbox items as handled.
func (c *Client) Ack(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/inbox/ack", map[string]any{"ids": ids}, nil)
}

// Send posts a message. clientMsgID makes retries safe.
func (c *Client) Send(ctx context.Context, channelID, threadRootID, body, clientMsgID string) error {
	return c.SendLong(ctx, channelID, threadRootID, body, clientMsgID, false)
}

// SendLong posts a message; elaborate allows an agent's message to be
// longer than the usual limit (a report someone asked for).
func (c *Client) SendLong(ctx context.Context, channelID, threadRootID, body, clientMsgID string, elaborate bool) error {
	in := map[string]any{"body": body, "client_msg_id": clientMsgID}
	if elaborate {
		in["elaborate"] = true
	}
	if threadRootID != "" {
		in["thread_root_id"] = threadRootID
	}
	return c.do(ctx, http.MethodPost, "/api/v1/channels/"+channelID+"/messages", in, nil)
}

// React adds a reaction.
func (c *Client) React(ctx context.Context, messageID, emoji string) error {
	return c.do(ctx, http.MethodPut, "/api/v1/messages/"+messageID+"/reactions/"+url.PathEscape(emoji), nil, nil)
}

// Thread returns a thread's root and replies.
func (c *Client) Thread(ctx context.Context, messageID string) (Message, []Message, error) {
	var r struct {
		Root    Message   `json:"root"`
		Replies []Message `json:"replies"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/messages/"+messageID+"/thread", nil, &r)
	return r.Root, r.Replies, err
}

// Handles maps actor IDs to handles (for thread context).
func (c *Client) Handles(ctx context.Context) (map[string]string, error) {
	var r struct {
		Actors []struct{ ID, Handle string }
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/actors", nil, &r); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(r.Actors))
	for _, a := range r.Actors {
		out[a.ID] = a.Handle
	}
	return out, nil
}

// HostAgent is one agent of the host config.
type HostAgent struct {
	AgentID        string  `json:"agent_id"`
	Handle         string  `json:"handle"`
	Client         string  `json:"client"`
	Model          string  `json:"model"`
	WorkModel      string  `json:"work_model"`
	AllowWork      bool    `json:"allow_work"`
	PermissionMode string  `json:"permission_mode"`
	Mode           string  `json:"mode"`
	RootDir        string  `json:"root_dir"`
	Sandbox        bool    `json:"sandbox"`
	Grants         []Grant `json:"grants"`
}

// HostConfig is what the server says this machine runs.
type HostConfig struct {
	Version string      `json:"version"`
	Agents  []HostAgent `json:"agents"`
}

// HostReport is what the guard tells the server about this machine.
type HostReport struct {
	GuardVersion     string         `json:"guard_version"`
	OS               string         `json:"os"`
	Roots            []string       `json:"roots"`
	Clients          []ReportClient `json:"clients"`
	Sandbox          string         `json:"sandbox"`
	AllowUnsandboxed bool           `json:"allow_unsandboxed"`
	Rejected         []Rejected     `json:"rejected"`
}

// ReportClient is an AI client found on this machine.
type ReportClient struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Rejected is a grant this machine refused, with the reason.
type Rejected struct {
	AgentID string `json:"agent_id"`
	Path    string `json:"path"`
	Reason  string `json:"reason"`
}

// Enroll registers this machine with a pairing code; it returns the host
// token (call on a Client without a token).
func (c *Client) Enroll(ctx context.Context, code, name string) (string, error) {
	var r struct {
		Token string `json:"token"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/hosts/enroll", map[string]any{"code": code, "name": name}, &r)
	return r.Token, err
}

// Report sends the machine report (host token).
func (c *Client) Report(ctx context.Context, r HostReport) error {
	return c.do(ctx, http.MethodPost, "/api/v1/host/report", r, nil)
}

// HostConfig fetches the config; changed is false when version is current.
func (c *Client) HostConfig(ctx context.Context, version string) (cfg HostConfig, changed bool, err error) {
	hdr := map[string]string{}
	if version != "" {
		hdr["If-None-Match"] = `"` + version + `"`
	}
	_, err = c.doH(ctx, http.MethodGet, "/api/v1/host/config", hdr, nil, &cfg)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotModified {
		return HostConfig{Version: version}, false, nil
	}
	return cfg, err == nil, err
}

// AgentToken requests a short-lived token for an assigned agent.
func (c *Client) AgentToken(ctx context.Context, agentID string) (string, time.Time, error) {
	var r struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/host/agents/"+agentID+"/token", nil, &r)
	return r.Token, r.ExpiresAt, err
}

// ReportError stores a run error's details on the server and returns its
// id; only humans can read them.
func (c *Client) ReportError(ctx context.Context, channelID, threadRootID, summary, detail string) (string, error) {
	in := map[string]any{"channel_id": channelID, "summary": summary, "detail": detail}
	if threadRootID != "" {
		in["thread_root_id"] = threadRootID
	}
	var r struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/agent/errors", in, &r)
	return r.ID, err
}
