// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The Workshop service with in-memory stores, a fake MCP server and fake
// approvals (round 56a). It lives here because a module may not import
// its own adapters (depguard).

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	evmem "colonyhall/internal/eventlog/memory"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/secrets"
	secretsmem "colonyhall/internal/modules/secrets/memory"
	"colonyhall/internal/modules/workshop"
	workshopmem "colonyhall/internal/modules/workshop/memory"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

type fakeRemote struct {
	mu     sync.Mutex
	tools  []workshop.ToolInfo
	calls  []string
	header []string // Authorization headers seen
	answer string
	err    error
}

func (f *fakeRemote) ListTools(_ context.Context, ep workshop.Endpoint) ([]workshop.ToolInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.header = append(f.header, ep.Header["Authorization"])
	return f.tools, f.err
}

func (f *fakeRemote) CallTool(_ context.Context, ep workshop.Endpoint, name string, args json.RawMessage) (workshop.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.header = append(f.header, ep.Header["Authorization"])
	f.calls = append(f.calls, name+" "+string(args))
	return workshop.Result{Text: f.answer}, f.err
}

type fakeApprovals struct {
	state   string
	created []approval.CreateInput
}

func (f *fakeApprovals) CreateRequest(_ context.Context, _ identity.Actor, in approval.CreateInput) (approval.ApprovalRequest, error) {
	f.created = append(f.created, in)
	return approval.ApprovalRequest{ID: ids.V7{}.New(), State: approval.StatePending}, nil
}

func (f *fakeApprovals) WaitForDecision(_ context.Context, req approval.ApprovalRequest, _ func(context.Context) (identity.Actor, error)) (approval.ApprovalRequest, error) {
	req.State = f.state
	return req, nil
}

type svcDir map[ids.ID]identity.Actor

func (d svcDir) ActorByID(_ context.Context, id ids.ID) (identity.Actor, error) {
	a, ok := d[id]
	if !ok {
		return identity.Actor{}, db.ErrNotFound
	}
	return a, nil
}

type svcEnv struct {
	svc     *workshop.Service
	remote  *fakeRemote
	appr    *fakeApprovals
	owner   identity.Actor
	admin   identity.Actor
	member  identity.Actor
	agent   identity.Actor
	other   identity.Actor // another workspace's agent
	ctx     context.Context
	key1    string // the k1 key, for rotation
	clock   *clock.Manual
	store   *workshopmem.Store
	secrets *secrets.Service
}

func newSvcEnv(t *testing.T, withKeys bool) *svcEnv {
	t.Helper()
	ws, ws2 := ids.V7{}.New(), ids.V7{}.New()
	e := &svcEnv{remote: &fakeRemote{answer: "3 issues"}, appr: &fakeApprovals{state: approval.StateAllowed},
		clock: clock.NewManual(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)), store: workshopmem.New()}
	mk := func(w ids.ID, kind identity.Kind, role identity.Role, h string) identity.Actor {
		return identity.Actor{ID: ids.V7{}.New(), WorkspaceID: w, Kind: kind, Role: role, Handle: h, State: identity.StateActive}
	}
	e.owner = mk(ws, identity.KindHuman, identity.RoleOwner, "norbi")
	e.admin = mk(ws, identity.KindHuman, identity.RoleAdmin, "anna")
	e.member = mk(ws, identity.KindHuman, identity.RoleMember, "bela")
	e.agent = mk(ws, identity.KindAgent, identity.RoleMember, "bot")
	e.other = mk(ws2, identity.KindAgent, identity.RoleMember, "intruder")
	d := svcDir{}
	for _, a := range []identity.Actor{e.owner, e.admin, e.member, e.agent, e.other} {
		d[a.ID] = a
	}
	var keys *secretbox.Keyring
	if withKeys {
		k, err := secretbox.NewKey("k1")
		if err != nil {
			t.Fatal(err)
		}
		if keys, err = secretbox.Parse(k); err != nil {
			t.Fatal(err)
		}
		e.key1 = k
	}
	events := evmem.New()
	tx := &db.Memory{}
	e.secrets = &secrets.Service{Store: secretsmem.New(), Keys: keys, Events: events, Tx: tx, IDs: ids.V7{}, Clock: e.clock}
	e.svc = &workshop.Service{Store: e.store, Secrets: e.secrets, Remote: e.remote, Approvals: e.appr, Directory: d,
		Events: events, Tx: tx, IDs: ids.V7{}, Clock: e.clock}
	e.ctx = db.WithWorkspace(context.Background(), ws)
	e.remote.tools = []workshop.ToolInfo{
		{Name: "search", Description: "Search", InputSchema: json.RawMessage(`{"type":"object"}`), Risk: workshop.RiskRead},
		{Name: "create_issue", Description: "Create", InputSchema: json.RawMessage(`{"type":"object"}`), Risk: workshop.RiskWrite},
		{Name: "bad__name", Risk: workshop.RiskRead},
	}
	return e
}

func str(s string) *string { return &s }

func (e *svcEnv) connector(t *testing.T) workshop.Connector {
	t.Helper()
	c, err := e.svc.CreateConnector(e.ctx, e.admin, workshop.ConnectorInput{Slug: "jira", Title: str("Jira"), Kind: workshop.KindMCP,
		BaseURL: str("https://mcp.example.com/mcp"), Auth: str(workshop.AuthBearer), Secret: str("tok-1234567890abcd")})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *svcEnv) current(a identity.Actor) func(context.Context) (identity.Actor, error) {
	return func(context.Context) (identity.Actor, error) { return a, nil }
}

func TestOnlyAdminsManageConnectors(t *testing.T) {
	e := newSvcEnv(t, true)
	for _, by := range []identity.Actor{e.member, e.agent} {
		_, err := e.svc.CreateConnector(e.ctx, by, workshop.ConnectorInput{Slug: "jira", Title: str("Jira"), Kind: workshop.KindMCP,
			BaseURL: str("https://x.example.com")})
		if !errors.Is(err, workshop.ErrForbidden) {
			t.Fatalf("%s created a connector: %v", by.Handle, err)
		}
		if _, err := e.svc.ListConnectors(e.ctx, by); !errors.Is(err, workshop.ErrForbidden) {
			t.Fatalf("%s listed connectors: %v", by.Handle, err)
		}
	}
}

func TestCreateConnectorStoresSecretSealedAndFetchesTools(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if c.State != "ok" || len(c.Tools) != 2 { // bad__name is left out
		t.Fatalf("connector: %+v", c)
	}
	if e.remote.header[0] != "Bearer tok-1234567890abcd" {
		t.Fatalf("auth header = %q", e.remote.header[0])
	}
	views, err := e.svc.ListConnectors(e.ctx, e.admin)
	if err != nil || len(views) != 1 || views[0].SecretHint != "abcd" || !views[0].HasSecret {
		t.Fatalf("views: %+v %v", views, err)
	}
	// The view never carries the value.
	b, _ := json.Marshal(views)
	if strings.Contains(string(b), "tok-1234567890abcd") {
		t.Fatal("the secret leaked into the view")
	}
}

func TestConnectorValidation(t *testing.T) {
	e := newSvcEnv(t, true)
	cases := []workshop.ConnectorInput{
		{Slug: "Jira", Title: str("x"), Kind: workshop.KindMCP, BaseURL: str("https://x.example.com")},
		{Slug: "ji__ra", Title: str("x"), Kind: workshop.KindMCP, BaseURL: str("https://x.example.com")},
		{Slug: "jira", Title: str(""), Kind: workshop.KindMCP, BaseURL: str("https://x.example.com")},
		{Slug: "jira", Title: str("x"), Kind: "ftp", BaseURL: str("https://x.example.com")},
		{Slug: "jira", Title: str("x"), Kind: workshop.KindMCP, BaseURL: str("ftp://x.example.com")},
		{Slug: "jira", Title: str("x"), Kind: workshop.KindMCP, BaseURL: str("https://x.example.com"), Auth: str(workshop.AuthBearer)},
		{Slug: "jira", Title: str("x"), Kind: workshop.KindMCP, BaseURL: str("https://x.example.com"), Auth: str(workshop.AuthHeader), HeaderName: str("Host"), Secret: str("v")},
	}
	for i, in := range cases {
		if _, err := e.svc.CreateConnector(e.ctx, e.admin, in); err == nil {
			t.Errorf("case %d accepted: %+v", i, in)
		}
	}
	e.connector(t)
	if _, err := e.svc.CreateConnector(e.ctx, e.admin, workshop.ConnectorInput{Slug: "jira", Title: str("x"), Kind: workshop.KindMCP,
		BaseURL: str("https://x.example.com")}); !errors.Is(err, workshop.ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
}

func TestSecretNeedsKeys(t *testing.T) {
	e := newSvcEnv(t, false)
	_, err := e.svc.CreateConnector(e.ctx, e.admin, workshop.ConnectorInput{Slug: "jira", Title: str("Jira"), Kind: workshop.KindMCP,
		BaseURL: str("https://mcp.example.com"), Auth: str(workshop.AuthBearer), Secret: str("tok")})
	if !errors.Is(err, secrets.ErrNoKeys) {
		t.Fatalf("err = %v, want ErrNoKeys", err)
	}
	// Without a secret it works.
	if _, err := e.svc.CreateConnector(e.ctx, e.admin, workshop.ConnectorInput{Slug: "open", Title: str("Open"), Kind: workshop.KindMCP,
		BaseURL: str("https://mcp.example.com")}); err != nil {
		t.Fatal(err)
	}
}

func TestToolsOnlyWithGrant(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	tools, err := e.svc.ToolsFor(e.ctx, e.agent)
	if err != nil || len(tools) != 0 {
		t.Fatalf("tools without a grant: %+v %v", tools, err)
	}
	_, err = e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__search", Current: e.current(e.agent)})
	if !errors.Is(err, workshop.ErrNoGrant) {
		t.Fatalf("call without a grant: %v", err)
	}
	if _, err := e.svc.AddGrant(e.ctx, e.member, e.agent.ID, workshop.TargetConnector, c.ID); !errors.Is(err, workshop.ErrForbidden) {
		t.Fatalf("a member granted: %v", err)
	}
	if _, err := e.svc.AddGrant(e.ctx, e.admin, e.member.ID, workshop.TargetConnector, c.ID); !errors.Is(err, workshop.ErrNotAgent) {
		t.Fatalf("a human got a grant: %v", err)
	}
	if _, err := e.svc.AddGrant(e.ctx, e.admin, e.other.ID, workshop.TargetConnector, c.ID); !errors.Is(err, workshop.ErrNotFound) {
		t.Fatalf("another workspace's agent got a grant: %v", err)
	}
	g, err := e.svc.AddGrant(e.ctx, e.admin, e.agent.ID, workshop.TargetConnector, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.AddGrant(e.ctx, e.admin, e.agent.ID, workshop.TargetConnector, c.ID)
	if err != nil || again.ID != g.ID {
		t.Fatalf("granting twice: %+v %v", again, err)
	}
	tools, _ = e.svc.ToolsFor(e.ctx, e.agent)
	if len(tools) != 2 || tools[0].Name != "jira__search" || tools[1].Name != "jira__create_issue" {
		t.Fatalf("tools: %+v", tools)
	}
	if !strings.Contains(tools[1].Description, "approval") || strings.Contains(tools[0].Description, "approval") {
		t.Fatalf("the write tool must say it needs approval: %+v", tools)
	}
	res, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__search", Args: json.RawMessage(`{"q":"x"}`), Current: e.current(e.agent)})
	if err != nil || res.Text != "3 issues" {
		t.Fatalf("read call: %+v %v", res, err)
	}
	if len(e.appr.created) != 0 {
		t.Fatal("a read call asked for approval")
	}
	if err := e.svc.RevokeGrant(e.ctx, e.admin, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__search", Current: e.current(e.agent)}); !errors.Is(err, workshop.ErrNoGrant) {
		t.Fatalf("call after revoke: %v", err)
	}
}

func TestWriteCallNeedsApproval(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if _, err := e.svc.AddGrant(e.ctx, e.admin, e.agent.ID, workshop.TargetConnector, c.ID); err != nil {
		t.Fatal(err)
	}
	ch := ids.V7{}.New()
	// No conversation: refused, nothing called.
	if _, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__create_issue", Current: e.current(e.agent)}); !errors.Is(err, workshop.ErrNoChannel) {
		t.Fatalf("write without a conversation: %v", err)
	}
	// Denied: not called.
	e.appr.state = approval.StateDenied
	res, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__create_issue", ChannelID: ch, Current: e.current(e.agent)})
	if err != nil || !res.IsError || len(e.remote.calls) != 0 {
		t.Fatalf("denied call: %+v %v calls=%v", res, err, e.remote.calls)
	}
	// Allowed: called once.
	e.appr.state = approval.StateAllowed
	if _, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__create_issue", Args: json.RawMessage(`{}`), ChannelID: ch, Current: e.current(e.agent)}); err != nil {
		t.Fatal(err)
	}
	if len(e.remote.calls) != 1 || len(e.appr.created) != 2 || e.appr.created[1].ChannelID != ch {
		t.Fatalf("calls=%v approvals=%+v", e.remote.calls, e.appr.created)
	}
	// Only the owner switches approval off; then no approval is asked.
	if _, err := e.svc.SetApprovalOff(e.ctx, e.admin, c.ID, true); !errors.Is(err, workshop.ErrForbidden) {
		t.Fatalf("an admin switched approval off: %v", err)
	}
	if _, err := e.svc.SetApprovalOff(e.ctx, e.owner, c.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__create_issue", Current: e.current(e.agent)}); err != nil {
		t.Fatalf("approval off: %v", err)
	}
	if len(e.appr.created) != 2 {
		t.Fatal("approval asked although it is off")
	}
	// A risk override makes a read tool ask again (approval on).
	if _, err := e.svc.SetApprovalOff(e.ctx, e.owner, c.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetRisk(e.ctx, e.admin, c.ID, "search", workshop.RiskExternal); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__search", ChannelID: ch, Current: e.current(e.agent)}); err != nil {
		t.Fatal(err)
	}
	if len(e.appr.created) != 3 {
		t.Fatal("the overridden risk did not ask")
	}
	calls, err := e.svc.ListCalls(e.ctx, e.admin, nil, nil, 50)
	if err != nil || len(calls) != 5 {
		t.Fatalf("call log: %d %v", len(calls), err)
	}
	if calls[len(calls)-1].Outcome != workshop.OutcomeDenied {
		t.Fatalf("the first call is logged as denied: %+v", calls[len(calls)-1])
	}
}

func TestHaltedAgentCannotCall(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if _, err := e.svc.AddGrant(e.ctx, e.admin, e.agent.ID, workshop.TargetConnector, c.ID); err != nil {
		t.Fatal(err)
	}
	halted := e.agent
	halted.Halted = true
	if _, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__search", Current: e.current(halted)}); !errors.Is(err, identity.ErrHalted) {
		t.Fatalf("halted agent: %v", err)
	}
	if len(e.remote.calls) != 0 {
		t.Fatal("a halted agent reached the connector")
	}
}

func TestLongResultIsCut(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if _, err := e.svc.AddGrant(e.ctx, e.admin, e.agent.ID, workshop.TargetConnector, c.ID); err != nil {
		t.Fatal(err)
	}
	e.remote.answer = strings.Repeat("é", workshop.MaxResult)
	res, err := e.svc.Call(e.ctx, workshop.CallInput{Name: "jira__search", Current: e.current(e.agent)})
	if err != nil || len(res.Text) > workshop.MaxResult+40 || !strings.Contains(res.Text, "bytes cut") {
		t.Fatalf("len=%d err=%v", len(res.Text), err)
	}
}

func TestDeleteConnectorRevokesGrantsAndSecret(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if _, err := e.svc.AddGrant(e.ctx, e.admin, e.agent.ID, workshop.TargetConnector, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteConnector(e.ctx, e.admin, c.ID); err != nil {
		t.Fatal(err)
	}
	if gs, _ := e.svc.ListGrants(e.ctx, e.admin); len(gs) != 0 {
		t.Fatalf("grants left: %+v", gs)
	}
	if _, err := e.secrets.Reveal(e.ctx, c.WorkspaceID, c.SecretID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("secret left: %v", err)
	}
}

func TestUpdateSecretRotatesAndRefreshes(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	before := len(e.remote.header)
	if _, err := e.svc.UpdateConnector(e.ctx, e.admin, c.ID, workshop.ConnectorInput{Secret: str("tok-new-0987654321wxyz")}); err != nil {
		t.Fatal(err)
	}
	if len(e.remote.header) != before+1 || e.remote.header[before] != "Bearer tok-new-0987654321wxyz" {
		t.Fatalf("headers: %v", e.remote.header)
	}
	// Switching to header auth needs a new secret.
	if _, err := e.svc.UpdateConnector(e.ctx, e.admin, c.ID, workshop.ConnectorInput{Auth: str(workshop.AuthHeader), HeaderName: str("X-Api-Key")}); !errors.Is(err, workshop.ErrSecretNeeded) {
		t.Fatalf("auth change without a secret: %v", err)
	}
}

// Rotation re-seals a connector's secret with the new active key; the old
// key may then be dropped.
func TestRotateWorkspaceSecrets(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	k2, err := secretbox.NewKey("k2")
	if err != nil {
		t.Fatal(err)
	}
	if e.secrets.Keys, err = secretbox.Parse(k2 + "," + e.key1); err != nil {
		t.Fatal(err)
	}
	n, err := e.secrets.Rotate(e.ctx)
	if err != nil || n != 1 {
		t.Fatalf("rotate: %d %v", n, err)
	}
	if n, _ := e.secrets.Rotate(e.ctx); n != 0 {
		t.Fatalf("second rotate: %d", n)
	}
	if e.secrets.Keys, err = secretbox.Parse(k2); err != nil {
		t.Fatal(err)
	}
	v, err := e.secrets.Reveal(e.ctx, c.WorkspaceID, c.SecretID)
	if err != nil || v != "tok-1234567890abcd" {
		t.Fatalf("after dropping the old key: %q %v", v, err)
	}
}

// Lowering a tool's risk skips the approval like switching it off: only
// the owner may (security review of round 56a). Raising is for admins too.
func TestLoweringRiskIsOwnerOnly(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if _, err := e.svc.SetRisk(e.ctx, e.admin, c.ID, "create_issue", workshop.RiskRead); !errors.Is(err, workshop.ErrOwnerOnly) {
		t.Fatalf("an admin lowered the risk: %v", err)
	}
	if _, err := e.svc.SetRisk(e.ctx, e.admin, c.ID, "search", workshop.RiskExternal); err != nil {
		t.Fatalf("an admin raises a risk: %v", err)
	}
	// Removing the raise lowers it again: owner only.
	if _, err := e.svc.SetRisk(e.ctx, e.admin, c.ID, "search", ""); !errors.Is(err, workshop.ErrOwnerOnly) {
		t.Fatalf("an admin removed a raise: %v", err)
	}
	if _, err := e.svc.SetRisk(e.ctx, e.owner, c.ID, "create_issue", workshop.RiskRead); err != nil {
		t.Fatalf("the owner lowers a risk: %v", err)
	}
}

// A new server address needs the secret again: the stored one is never
// sent to another host (security review of round 56a).
func TestNewOriginNeedsTheSecretAgain(t *testing.T) {
	e := newSvcEnv(t, true)
	c := e.connector(t)
	if _, err := e.svc.UpdateConnector(e.ctx, e.admin, c.ID, workshop.ConnectorInput{BaseURL: str("https://evil.example.net/mcp")}); !errors.Is(err, workshop.ErrSecretAgain) {
		t.Fatalf("moved to another host with the old secret: %v", err)
	}
	if _, err := e.svc.UpdateConnector(e.ctx, e.admin, c.ID, workshop.ConnectorInput{BaseURL: str("https://mcp.example.com/v2/mcp")}); err != nil {
		t.Fatalf("a new path on the same host: %v", err)
	}
	if _, err := e.svc.UpdateConnector(e.ctx, e.admin, c.ID, workshop.ConnectorInput{BaseURL: str("https://other.example.com/mcp"), Secret: str("tok-other-1234567890")}); err != nil {
		t.Fatalf("another host with a new secret: %v", err)
	}
}
