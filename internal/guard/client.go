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
	// WSTokenHook, when set, overrides the token used in the WebSocket hello frame (test hook).
	WSTokenHook func(ctx context.Context) (string, error)
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
	DelegatedBy  string    `json:"delegated_by,omitempty"`
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

// SetActivity reports whether the agent is currently working or idle in a channel/thread.
func (c *Client) SetActivity(ctx context.Context, channelID, threadRootID, state string) error {
	in := map[string]string{
		"channel_id": channelID,
		"state":      state,
	}
	if threadRootID != "" {
		in["thread_root_id"] = threadRootID
	}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/activity", in, nil)
}

// ReportProgress sends progress steps to the server during a run.
func (c *Client) ReportProgress(ctx context.Context, channelID, threadRootID, runKey string, steps []Step, done bool) error {
	in := map[string]any{
		"channel_id": channelID,
		"run_key":    runKey,
		"steps":      steps,
		"done":       done,
	}
	if threadRootID != "" {
		in["thread_root_id"] = threadRootID
	}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/progress", in, nil)
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
	_, err := c.SendLong(ctx, channelID, threadRootID, body, clientMsgID, false)
	return err
}

// SendLong posts a message; elaborate allows an agent's message to be
// longer than the usual limit (a report someone asked for). It returns the created message ID if available.
func (c *Client) SendLong(ctx context.Context, channelID, threadRootID, body, clientMsgID string, elaborate bool) (string, error) {
	in := map[string]any{"body": body, "client_msg_id": clientMsgID}
	if elaborate {
		in["elaborate"] = true
	}
	if threadRootID != "" {
		in["thread_root_id"] = threadRootID
	}
	var res struct {
		Message *struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/channels/"+channelID+"/messages", in, &res)
	if err != nil {
		return "", err
	}
	if res.Message != nil {
		return res.Message.ID, nil
	}
	return "", nil
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

// HasDelegation checks whether a delegation exists on the server for messageID.
func (c *Client) HasDelegation(ctx context.Context, messageID string) (bool, string, error) {
	if messageID == "" {
		return false, "", nil
	}
	var res struct {
		Delegations []struct {
			State           string  `json:"state"`
			DecidedByHandle *string `json:"decided_by_handle,omitempty"`
			ApproverHandle  *string `json:"approver_handle,omitempty"`
		} `json:"delegations"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/delegations?message_id="+url.QueryEscape(messageID), nil, &res)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && (ae.Status == http.StatusForbidden || ae.Status == http.StatusNotFound) {
			return false, "", nil
		}
		return false, "", err
	}
	if len(res.Delegations) == 0 {
		return false, "", nil
	}
	approver := "norbi"
	d := res.Delegations[0]
	if d.ApproverHandle != nil && *d.ApproverHandle != "" {
		approver = *d.ApproverHandle
	} else if d.DecidedByHandle != nil && *d.DecidedByHandle != "" {
		approver = *d.DecidedByHandle
	}
	return true, approver, nil
}

// HostAgent is one agent of the host config.
type HostAgent struct {
	AgentID          string  `json:"agent_id"`
	Handle           string  `json:"handle"`
	Client           string  `json:"client"`
	Model            string  `json:"model"`
	WorkModel        string  `json:"work_model"`
	AllowWork        bool    `json:"allow_work"`
	PermissionMode   string  `json:"permission_mode"`
	Mode             string  `json:"mode"`
	RootDir          string  `json:"root_dir"`
	Sandbox          bool    `json:"sandbox"`
	Approvals        string  `json:"approvals"`
	MaxSessionTokens int     `json:"max_session_tokens"`
	DailyTokenLimit  int64   `json:"daily_token_limit"`
	Grants           []Grant `json:"grants"`
	WorktreeRepo     string  `json:"worktree_repo"`
	AllowPreview     bool    `json:"allow_preview"`
	AllowWebRead     bool    `json:"allow_web_read"`
}

// HostPreview is one preview the guard needs to run or stop.
type HostPreview struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Name        string    `json:"name"`
	ComposePath string    `json:"compose_path"`
	State       string    `json:"state"`
	URL         string    `json:"url,omitempty"`
	Error       string    `json:"error,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// HostConfig is what the server says this machine runs.
type HostConfig struct {
	Version  string        `json:"version"`
	Agents   []HostAgent   `json:"agents"`
	Previews []HostPreview `json:"previews"`
}

// HostReport is what the guard tells the server about this machine.
type HostReport struct {
	GuardVersion     string         `json:"guard_version"`
	OS               string         `json:"os"`
	Roots            []string       `json:"roots"`
	Clients          []ReportClient `json:"clients"`
	Sandbox          string         `json:"sandbox"`
	AllowUnsandboxed bool           `json:"allow_unsandboxed"`
	ExchangeDir      string         `json:"exchange_dir,omitempty"`
	ExchangeOK       bool           `json:"exchange_ok"`
	Rejected         []Rejected     `json:"rejected"`
	Worktrees        []Worktree     `json:"worktrees,omitempty"`
}

// Worktree is an active git worktree on this machine.
type Worktree struct {
	AgentID string `json:"agent_id"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
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

// ReportRunInput is the payload sent when reporting an agent run.
type ReportRunInput struct {
	HostID           string   `json:"host_id,omitempty"`
	Phase            string   `json:"phase"`
	Client           string   `json:"client"`
	Model            string   `json:"model"`
	SessionID        string   `json:"session_id"`
	ChannelID        string   `json:"channel_id,omitempty"`
	ThreadRootID     string   `json:"thread_root_id,omitempty"`
	Items            int      `json:"items"`
	InputTokens      int64    `json:"input_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	DurationMS       int      `json:"duration_ms"`
	Status           string   `json:"status"`
	ErrorID          string   `json:"error_id,omitempty"`
	WebReads         []string `json:"web_reads,omitempty"`
	Steps            []Step   `json:"steps,omitempty"`
	ReplyMessageID   string   `json:"reply_message_id,omitempty"`
}

// ReportRun reports a finished run with token counts and duration to the server.
func (c *Client) ReportRun(ctx context.Context, in ReportRunInput) (string, error) {
	var r struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/agent/runs", in, &r)
	return r.ID, err
}

// AgentBudget represents the agent's daily token budget.
type AgentBudget struct {
	Limit     int64     `json:"limit"`
	UsedToday int64     `json:"used_today"`
	Remaining int64     `json:"remaining"`
	ResetsAt  time.Time `json:"resets_at"`
}

// Budget fetches the agent's daily token budget from the server.
func (c *Client) Budget(ctx context.Context) (AgentBudget, error) {
	var b AgentBudget
	err := c.do(ctx, http.MethodGet, "/api/v1/agent/budget", nil, &b)
	return b, err
}

// ReportPreview sends the preview status to the server.
func (c *Client) ReportPreview(ctx context.Context, previewID, state, url, errStr string) error {
	in := map[string]any{"state": state}
	if url != "" {
		in["url"] = url
	}
	if errStr != "" {
		in["error"] = errStr
	}
	return c.do(ctx, http.MethodPost, "/api/v1/host/previews/"+previewID, in, nil)
}

// AgentInstructions represents the rendered instructions for the calling agent.
type AgentInstructions struct {
	Version int    `json:"version"`
	Text    string `json:"text"`
}

// Instructions fetches the agent's instructions (rendered rules).
func (c *Client) Instructions(ctx context.Context) (AgentInstructions, error) {
	var inst AgentInstructions
	err := c.do(ctx, http.MethodGet, "/api/v1/agent/instructions", nil, &inst)
	return inst, err
}

// AgentNoteItem represents one note row returned by GET /api/v1/agents/{id}/notes.
type AgentNoteItem struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	ThreadRootID string    `json:"thread_root_id"`
	Text         string    `json:"text"`
	CreatedAt    time.Time `json:"created_at"`
}

// AgentNotes returns notes for agentID, optionally filtered by thread.
func (c *Client) AgentNotes(ctx context.Context, agentID, threadRootID string) ([]AgentNoteItem, error) {
	path := "/api/v1/agents/" + agentID + "/notes"
	if threadRootID != "" {
		path += "?thread=" + url.QueryEscape(threadRootID)
	}
	var r struct {
		Notes []AgentNoteItem `json:"notes"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &r)
	return r.Notes, err
}

// LatestNote returns the newest note text for agentID in threadRootID, or "" if none.
func (c *Client) LatestNote(ctx context.Context, agentID, threadRootID string) (string, error) {
	notes, err := c.AgentNotes(ctx, agentID, threadRootID)
	if err != nil {
		return "", err
	}
	if len(notes) == 0 {
		return "", nil
	}
	return notes[0].Text, nil
}

// AgentPendingFileFetch describes a file fetch request queued for an agent.
type AgentPendingFileFetch struct {
	ID            string `json:"id"`
	MessageID     string `json:"message_id"`
	ChannelID     string `json:"channel_id"`
	Path          string `json:"path"`
	MaxImageBytes int64  `json:"max_image_bytes"`
	MaxFileBytes  int64  `json:"max_file_bytes"`
}

// PendingFileFetches retrieves pending file fetch requests for this agent.
func (c *Client) PendingFileFetches(ctx context.Context) ([]AgentPendingFileFetch, error) {
	var out []AgentPendingFileFetch
	err := c.do(ctx, http.MethodGet, "/api/v1/agent/file-fetch?state=pending", nil, &out)
	return out, err
}

// UploadFetchedContent streams fetched file content to the server.
// uploadTimeout bounds one fetched-file upload.
const uploadTimeout = 110 * time.Second

func (c *Client) UploadFetchedContent(ctx context.Context, id, contentType, filename, mtime string, content io.Reader, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimSuffix(c.BaseURL, "/")+"/api/v1/agent/file-fetch/"+id+"/content", content)
	if err != nil {
		return err
	}
	token := c.Token
	if c.Tokens != nil {
		if token, err = c.Tokens.Token(ctx); err != nil {
			return err
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	if filename != "" {
		req.Header.Set("X-File-Name", filename)
	}
	if mtime != "" {
		req.Header.Set("X-File-Mtime", mtime)
	}
	if size >= 0 {
		req.ContentLength = size
	}
	// The client's overall timeout (30 s) would cut a large file short; the
	// server fails an unanswered request after 2 minutes, so stop before that.
	hc := &http.Client{}
	if c.HTTP != nil {
		copied := *c.HTTP
		hc = &copied
	}
	hc.Timeout = 0
	upCtx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	req = req.WithContext(upCtx)
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusUnauthorized && c.Tokens != nil {
		c.Tokens.Invalidate()
	}
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		var p struct{ Detail, Title string }
		_ = json.Unmarshal(raw, &p)
		if p.Detail == "" {
			p.Detail = p.Title
		}
		return &APIError{Status: res.StatusCode, Detail: p.Detail}
	}
	return nil
}

// FailFileFetch reports a file fetch request failure with an error code.
func (c *Client) FailFileFetch(ctx context.Context, id, errorCode string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/agent/file-fetch/"+id+"/fail", map[string]string{
		"error_code": errorCode,
	}, nil)
}

// ScribeTaskMessage is a message in a compaction task.
type ScribeTaskMessage struct {
	ID           string    `json:"id"`
	AuthorHandle string    `json:"author_handle"`
	AuthorKind   string    `json:"author_kind"`
	Body         string    `json:"body"`
	Attachments  []string  `json:"attachments,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ScribeCompactionTask is the payload returned by GET /api/v1/agent/compactions/{id}.
type ScribeCompactionTask struct {
	ID              string              `json:"id"`
	ChannelID       string              `json:"channel_id"`
	ThreadRootID    string              `json:"thread_root_id,omitempty"`
	PreviousSummary string              `json:"previous_summary,omitempty"`
	Messages        []ScribeTaskMessage `json:"messages"`
	MaxSummaryChars int                 `json:"max_summary_chars,omitempty"`
}

// CompactionTask queries the compaction task details for the scribe agent.
func (c *Client) CompactionTask(ctx context.Context, id string) (ScribeCompactionTask, error) {
	var task ScribeCompactionTask
	err := c.do(ctx, http.MethodGet, "/api/v1/agent/compactions/"+id, nil, &task)
	return task, err
}

// ReportCompactionResult posts the scribe agent's summary or error code.
func (c *Client) ReportCompactionResult(ctx context.Context, id, markdown, errorCode string) error {
	in := map[string]any{}
	if markdown != "" {
		in["markdown"] = markdown
	}
	if errorCode != "" {
		in["error_code"] = errorCode
	}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/compactions/"+id+"/result", in, nil)
}

// ContextMessage is a message in an agent context.
type ContextMessage struct {
	ID           string    `json:"id"`
	AuthorHandle string    `json:"author_handle"`
	AuthorKind   string    `json:"author_kind"`
	Body         string    `json:"body"`
	Attachments  []string  `json:"attachments,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// AgentContext is the payload returned by GET /api/v1/agent/context.
type AgentContext struct {
	CompactionID    string           `json:"compaction_id,omitempty"`
	SummaryMarkdown string           `json:"summary_markdown,omitempty"`
	MessagesAfter   []ContextMessage `json:"messages_after"`
	HasMore         bool             `json:"has_more"`
}

// AgentContext fetches the primed context for a channel/thread.
func (c *Client) AgentContext(ctx context.Context, channelID, threadRootID string) (AgentContext, error) {
	path := "/api/v1/agent/context?channel_id=" + url.QueryEscape(channelID)
	if threadRootID != "" {
		path += "&thread_root_id=" + url.QueryEscape(threadRootID)
	}
	var res AgentContext
	err := c.do(ctx, http.MethodGet, path, nil, &res)
	return res, err
}
