// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/extrunner"
	"colonyhall/internal/platform/ids"
)

// Extension states.
const (
	StateDraft    = "draft"    // no approved version yet
	StateLive     = "live"     // its live version answers calls
	StateDisabled = "disabled" // switched off (by a person or after errors)
)

// Tool kinds.
const (
	ToolHTTP = "http"
	ToolJS   = "js"
)

// Limits of the extensions.
const (
	MaxSource      = 256 * 1024
	MaxExtTools    = 30
	MaxStoreDoc    = 64 * 1024
	MaxStoreDocs   = 10000
	MaxStoreBytes  = 10 * 1024 * 1024
	MaxCollections = 10
	MaxEgress      = 20
	// GatewayPrefix marks an extension's tools: x_<slug>__<tool>.
	GatewayPrefix = "x_"
)

// Errors.
var (
	ErrBadManifest     = errors.New("invalid manifest")
	ErrNoLiveVersion   = errors.New("the extension has no live version")
	ErrNotLive         = errors.New("the extension is switched off")
	ErrVersionNotFound = errors.New("no such version")
	ErrNotApproved     = errors.New("only an approved version can be made live again")
	ErrMaintainerGone  = errors.New("the extension's maintainer is no longer an owner or admin: an admin has to take it over")
	ErrBudget          = errors.New("the workspace's monthly call budget is used up")
	ErrBusy            = errors.New("too many tool calls are running in this workspace; try again shortly")
	ErrStoreFull       = errors.New("the extension's store is full")
	ErrCompile         = errors.New("the code does not compile")
)

// Limits are the server owner's numbers for code tools (COLONY_EXT_*).
type Limits struct {
	Run                  extrunner.Limits
	ConcurrencyWorkspace int
	DisableConsecutive   int
	DisableErrors        int
	DisableWindow        time.Duration
}

// DefaultLimits are docs/25 13.4's numbers.
var DefaultLimits = Limits{Run: extrunner.DefaultLimits, ConcurrencyWorkspace: 4, DisableConsecutive: 5, DisableErrors: 10,
	DisableWindow: 10 * time.Minute}

// Extension is a workspace's own set of tools.
type Extension struct {
	ID                ids.ID
	WorkspaceID       ids.ID
	Slug              string
	Title             string
	Description       string
	MaintainerID      ids.ID
	BotID             ids.ID
	State             string
	LiveVersion       int // 0: none
	ApprovalOff       bool
	DisabledReason    string
	ConsecutiveErrors int
	RecentErrors      []time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

// Version is one immutable version of an extension.
type Version struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	ExtensionID  ids.ID
	Version      int
	Manifest     Manifest
	Source       string
	Bundle       string
	BundleSHA256 string
	CreatedBy    ids.ID
	CreatedAt    time.Time
}

// Approval records who approved a version and what it added.
type Approval struct {
	ID          ids.ID
	WorkspaceID ids.ID
	ExtensionID ids.ID
	Version     int
	DecidedBy   ids.ID
	Diff        Diff
	CreatedAt   time.Time
}

// StoreDoc is one document of an extension's store.
type StoreDoc struct {
	WorkspaceID ids.ID
	ExtensionID ids.ID
	Collection  string
	DocID       string
	Data        json.RawMessage
	SizeBytes   int
	UpdatedAt   time.Time
}

// Manifest describes an extension version (docs/25 3.).
type Manifest struct {
	Description string         `json:"description,omitempty"`
	Connectors  []string       `json:"connectors,omitempty"`
	Egress      []string       `json:"egress,omitempty"`
	Store       []string       `json:"store,omitempty"`
	Tools       []ManifestTool `json:"tools"`
}

// ManifestTool is one tool of a manifest.
type ManifestTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Kind        string          `json:"kind"`
	Risk        string          `json:"risk"`
	Input       json.RawMessage `json:"input,omitempty"`
	// Fn is the exported function of a js tool (default: the name).
	Fn string `json:"fn,omitempty"`
	// Request and Output describe an http tool.
	Request *HTTPRequest `json:"request,omitempty"`
	Output  *OutputMap   `json:"output,omitempty"`
}

// HTTPRequest is a declarative tool's request; {{input.x}} is the only
// template.
type HTTPRequest struct {
	Connector string            `json:"connector"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Query     map[string]string `json:"query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      json.RawMessage   `json:"body,omitempty"`
}

// OutputMap picks the parts of a JSON answer the model gets.
type OutputMap struct {
	Pick     map[string]string `json:"pick,omitempty"`
	MaxItems int               `json:"max_items,omitempty"`
}

var (
	collectionRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	hostRe       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	fnRe         = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)
	headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

func manifestErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrBadManifest, fmt.Sprintf(format, a...))
}

// Normalize checks a manifest and fills its defaults. connectors are the
// workspace's live connectors by slug.
func (m *Manifest) Normalize(connectors map[string]Connector) error {
	if utf8.RuneCountInString(m.Description) > 2000 {
		return manifestErr("the description is longer than 2000 characters")
	}
	if len(m.Tools) == 0 || len(m.Tools) > MaxExtTools {
		return manifestErr("an extension has 1–%d tools", MaxExtTools)
	}
	seen := map[string]bool{}
	for _, c := range m.Connectors {
		if _, ok := connectors[c]; !ok {
			return manifestErr("unknown connector %q (add it in the Workshop first)", c)
		}
		if seen["c:"+c] {
			return manifestErr("connector %q listed twice", c)
		}
		seen["c:"+c] = true
	}
	if len(m.Egress) > MaxEgress {
		return manifestErr("at most %d egress hosts", MaxEgress)
	}
	for i, h := range m.Egress {
		h = strings.ToLower(strings.TrimSpace(h))
		if !hostRe.MatchString(h) {
			return manifestErr("egress %q is not a host name", h)
		}
		m.Egress[i] = h
	}
	if len(m.Store) > MaxCollections {
		return manifestErr("at most %d store collections", MaxCollections)
	}
	for _, c := range m.Store {
		if !collectionRe.MatchString(c) {
			return manifestErr("store collection %q: lower-case letters, digits and _", c)
		}
	}
	names := map[string]bool{}
	for i := range m.Tools {
		t := &m.Tools[i]
		if !ValidToolName(t.Name) {
			return manifestErr("tool name %q: 1–24 letters, digits, _ or -, without __", t.Name)
		}
		if names[t.Name] {
			return manifestErr("tool %q listed twice", t.Name)
		}
		names[t.Name] = true
		if utf8.RuneCountInString(t.Description) > 1000 {
			return manifestErr("tool %q: the description is longer than 1000 characters", t.Name)
		}
		if t.Risk == "" {
			t.Risk = RiskWrite
		}
		if !validRisk(t.Risk) {
			return manifestErr("tool %q: risk must be read, write or external", t.Name)
		}
		if len(t.Input) == 0 {
			t.Input = json.RawMessage(`{"type":"object"}`)
		}
		var schema map[string]any
		if json.Unmarshal(t.Input, &schema) != nil || schema["type"] != "object" {
			return manifestErr("tool %q: input must be a JSON schema of type object", t.Name)
		}
		switch t.Kind {
		case ToolJS:
			if t.Fn == "" {
				t.Fn = t.Name
			}
			if !fnRe.MatchString(t.Fn) {
				return manifestErr("tool %q: fn must be a JavaScript name", t.Name)
			}
			t.Request, t.Output = nil, nil
		case ToolHTTP:
			r := t.Request
			if r == nil {
				return manifestErr("tool %q: an http tool needs a request", t.Name)
			}
			c, ok := connectors[r.Connector]
			if !ok || !seen["c:"+r.Connector] {
				return manifestErr("tool %q: the request's connector must be one of the manifest's connectors", t.Name)
			}
			if c.Kind != KindHTTP {
				return manifestErr("tool %q: connector %q is an MCP server; use its tools directly", t.Name, r.Connector)
			}
			r.Method = strings.ToUpper(r.Method)
			switch r.Method {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				return manifestErr("tool %q: method must be GET, POST, PUT, PATCH or DELETE", t.Name)
			}
			if !strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, "://") || strings.Contains(r.Path, "..") {
				return manifestErr("tool %q: path must start with / and stay on the connector", t.Name)
			}
			for k := range r.Headers {
				if !headerNameRe.MatchString(k) || strings.EqualFold(k, "host") || strings.EqualFold(k, "authorization") {
					return manifestErr("tool %q: header %q is not allowed", t.Name, k)
				}
			}
			if len(r.Body) > 0 && !json.Valid(r.Body) {
				return manifestErr("tool %q: body must be JSON", t.Name)
			}
			t.Fn = ""
		default:
			return manifestErr("tool %q: kind must be http or js", t.Name)
		}
	}
	return nil
}

// HasJS reports whether the manifest has a code tool.
func (m Manifest) HasJS() bool {
	for _, t := range m.Tools {
		if t.Kind == ToolJS {
			return true
		}
	}
	return false
}

// Tool returns a manifest tool by name.
func (m Manifest) Tool(name string) (ManifestTool, bool) {
	for _, t := range m.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return ManifestTool{}, false
}

// Diff is what a version adds compared with the live one: what a person
// approves (docs/25 4.).
type Diff struct {
	NewConnectors []string          `json:"new_connectors,omitempty"`
	NewEgress     []string          `json:"new_egress,omitempty"`
	NewStore      []string          `json:"new_store,omitempty"`
	NewTools      []string          `json:"new_tools,omitempty"`
	RaisedRisks   map[string]string `json:"raised_risks,omitempty"`
	CodeChanged   bool              `json:"code_changed"`
}

func added(old, cur []string) []string {
	have := map[string]bool{}
	for _, x := range old {
		have[x] = true
	}
	var out []string
	for _, x := range cur {
		if !have[x] {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// DiffVersions compares a version with the live one (nil: none).
func DiffVersions(live *Version, v Version) Diff {
	var old Manifest
	oldSHA := ""
	if live != nil {
		old, oldSHA = live.Manifest, live.BundleSHA256
	}
	d := Diff{NewConnectors: added(old.Connectors, v.Manifest.Connectors), NewEgress: added(old.Egress, v.Manifest.Egress),
		NewStore: added(old.Store, v.Manifest.Store), CodeChanged: live == nil || oldSHA != v.BundleSHA256}
	for _, t := range v.Manifest.Tools {
		ot, ok := old.Tool(t.Name)
		if !ok {
			d.NewTools = append(d.NewTools, t.Name)
			continue
		}
		if riskRank(t.Risk) > riskRank(ot.Risk) {
			if d.RaisedRisks == nil {
				d.RaisedRisks = map[string]string{}
			}
			d.RaisedRisks[t.Name] = t.Risk
		}
		if !d.CodeChanged && (ot.Kind != t.Kind || !sameRequest(ot.Request, t.Request)) {
			d.CodeChanged = true
		}
	}
	sort.Strings(d.NewTools)
	return d
}

func sameRequest(a, b *HTTPRequest) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// ExtensionGatewayName is an extension tool's name in the gateway.
func ExtensionGatewayName(slug, tool string) string { return GatewayPrefix + slug + "__" + tool }
