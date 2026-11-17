// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package workshop is the Workshop ("Műhely", round 56,
// docs/25-onfejlesztes-terv.md): a workspace extends its own Colonyhall
// with connectors to external MCP servers and HTTP APIs, its own tools and
// routines. This part (56a) is the MCP gateway: connectors, per-agent
// grants, the risk of each tool, approvals for write and external calls,
// and the call log. The core (login, roles, tenancy, audit, quotas, the
// emergency stop) is never reachable from here.
package workshop

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"colonyhall/internal/platform/ids"
)

// Connector kinds.
const (
	KindMCP  = "mcp"
	KindHTTP = "http"
)

// Authentication of a connector's requests.
const (
	AuthNone   = "none"
	AuthBearer = "bearer" // Authorization: Bearer <secret>
	AuthHeader = "header" // <HeaderName>: <secret>
	AuthBasic  = "basic"  // Authorization: Basic base64(<secret>), the secret being "user:password"
)

// Risks of a tool (docs/25 6.).
const (
	RiskRead     = "read"
	RiskWrite    = "write"
	RiskExternal = "external"
)

// Grant targets.
const (
	TargetConnector = "connector"
	TargetExtension = "extension"
	TargetWorkshop  = "workshop"
)

// Call outcomes.
const (
	OutcomeOK      = "ok"
	OutcomeError   = "error"
	OutcomeDenied  = "denied"
	OutcomeTimeout = "timeout"
	OutcomeLimit   = "limit"
)

// Limits.
const (
	// MaxResult is how much of a tool's answer the model gets (docs/25 13.3).
	MaxResult = 32 * 1024
	// MaxToolName: a gateway name must fit the Claude API's 64 characters
	// with the client's "mcp__colonyhall__" prefix: slug (16) + "__" + tool (24).
	MaxToolName = 24
	// ToolsTTL is how long a connector's tool list is cached.
	ToolsTTL = 10 * time.Minute
	// CallTimeout bounds one connector call.
	CallTimeout = 60 * time.Second
	// MaxTools bounds the tools taken from one MCP server.
	MaxTools = 100
)

// Errors.
var (
	ErrForbidden    = errors.New("only the owner and the admins manage the Workshop")
	ErrNotFound     = errors.New("not found")
	ErrBadSlug      = errors.New("the short name must be 2–16 characters: lower-case letters, digits and _, starting with a letter, without __")
	ErrSlugTaken    = errors.New("a connector with this short name already exists")
	ErrBadTitle     = errors.New("the title must be 1–80 characters")
	ErrBadKind      = errors.New("the kind must be mcp or http")
	ErrBadAuth      = errors.New("the authentication must be none, bearer, header or basic")
	ErrBadHeader    = errors.New("a header name is needed for header authentication (letters, digits and -)")
	ErrSecretNeeded = errors.New("this authentication needs a secret")
	ErrBadRisk      = errors.New("the risk must be read, write or external")
	ErrNotAgent     = errors.New("grants are given to agents")
	ErrUnknownTool  = errors.New("unknown tool")
	ErrNoGrant      = errors.New("this agent may not use this tool")
	ErrNoChannel    = errors.New("this tool needs approval, but the call is not in a conversation (missing X-Colonyhall-Channel)")
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,15}$`)
var headerRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// ValidSlug reports whether s may name a connector or an extension.
func ValidSlug(s string) bool { return slugRe.MatchString(s) && !strings.Contains(s, "__") }

// Connector is an external MCP server or HTTP API of a workspace.
type Connector struct {
	ID            ids.ID
	WorkspaceID   ids.ID
	Slug          string
	Title         string
	Kind          string
	BaseURL       string
	Auth          string
	HeaderName    string
	SecretID      ids.ID // ids.Nil: none
	ApprovalOff   bool
	RiskOverrides map[string]string // tool name → risk
	Tools         []ToolInfo
	ToolsCachedAt *time.Time
	State         string // new, ok, error
	LastError     string
	CreatedBy     ids.ID
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

// ToolInfo is one tool of a connector as the gateway offers it.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	// Risk is what the server's annotations say (read, write, external).
	Risk string `json:"risk"`
}

// EffectiveRisk returns the tool's risk after the admin's override.
func (c Connector) EffectiveRisk(tool ToolInfo) string {
	if r, ok := c.RiskOverrides[tool.Name]; ok {
		return r
	}
	if tool.Risk == "" {
		return RiskWrite
	}
	return tool.Risk
}

// Tool returns the connector's tool by name.
func (c Connector) Tool(name string) (ToolInfo, bool) {
	for _, t := range c.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return ToolInfo{}, false
}

// GatewayName is the name a connector's tool has in the gateway.
func GatewayName(slug, tool string) string { return slug + "__" + tool }

// SplitGatewayName splits "<slug>__<tool>".
func SplitGatewayName(name string) (slug, tool string, ok bool) {
	i := strings.Index(name, "__")
	if i <= 0 || i+2 >= len(name) {
		return "", "", false
	}
	return name[:i], name[i+2:], true
}

// ValidToolName reports whether a tool's own name fits the gateway.
func ValidToolName(n string) bool {
	if n == "" || len(n) > MaxToolName || strings.Contains(n, "__") {
		return false
	}
	for _, r := range n {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func validRisk(r string) bool { return r == RiskRead || r == RiskWrite || r == RiskExternal }

func validAuth(a string) bool {
	return a == AuthNone || a == AuthBearer || a == AuthHeader || a == AuthBasic
}

// Grant lets one agent use a connector (or, from 56b, an extension or the
// Workshop's meta-tools).
type Grant struct {
	ID          ids.ID
	WorkspaceID ids.ID
	AgentID     ids.ID
	TargetKind  string
	TargetID    ids.ID // ids.Nil for the workshop target
	GrantedBy   ids.ID
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

// ToolCall is one line of the call log (no content).
type ToolCall struct {
	ID          ids.ID
	WorkspaceID ids.ID
	ActorID     ids.ID
	TargetKind  string
	TargetID    ids.ID
	Tool        string
	Risk        string
	ApprovalID  ids.ID
	StartedAt   time.Time
	DurationMS  int
	InBytes     int
	OutBytes    int
	Outcome     string
	Error       string
}

// checkBaseURL is the format check; the egress policy decides at call
// time whether the address may be reached.
func checkBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || len(raw) > 500 {
		return errors.New("the address must be an http(s) URL with a host, at most 500 characters")
	}
	return nil
}
