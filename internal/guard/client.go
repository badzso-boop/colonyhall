package guard

import (
	"bytes"
	"context"
	"encoding/json"
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
	HTTP    *http.Client
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
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var p struct{ Detail, Title string }
		_ = json.Unmarshal(raw, &p)
		if p.Detail == "" {
			p.Detail = p.Title
		}
		return &APIError{Status: res.StatusCode, Detail: p.Detail}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
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
	in := map[string]any{"body": body, "client_msg_id": clientMsgID}
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
