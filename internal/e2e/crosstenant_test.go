package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/fakeagent"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/planner"
	"colonyhall/internal/modules/preview"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
	"colonyhall/internal/storetest"
)

// The cross-tenant suite (round 45, docs/23-elkulonites.md): workspace beta
// calls every REST operation of api/openapi.yaml, every MCP tool and the
// WebSocket channels against workspace acme's objects, on its own
// subdomain and on acme's. Nothing may succeed on an acme object, no
// response may carry acme data, and acme's data must be intact afterwards.
// The operations and tools are read from the contract and the server, so a
// new one is covered without touching this file; a new kind of path
// parameter must be mapped to an acme object below (the suite fails until
// it is).

// acmeData marks every piece of acme content (bodies, titles, display
// names, notes); it must never appear in a response to beta. Names that
// beta sends as references (channel and resource names, handles) use
// acmeRef instead, since an error may echo them.
const (
	acmeData = "acmeprivatedata"
	acmeRef  = "acmeref"
)

// ctSuite holds the two workspaces and acme's objects.
type ctSuite struct {
	t          *testing.T
	acme, beta *env
	betaOwner  ids.ID
	betaWS     ids.ID
	betaAgent  string            // beta's agent token
	betaHost   string            // beta's host token
	acmeAgent  string            // acme's agent token
	acmeHost   string            // acme's host token
	obj        map[string]string // acme object ids by kind
	acmeIDs    []string          // every acme id beta must never see unless it sent it
	dial       func(e *env, path, token string) (*websocket.Conn, map[string]any)
	statuses   map[int]int // beta's answers on acme objects, for the log
}

func TestCrossTenantSuite(t *testing.T) { runCrossTenant(t, nil) }

// TestPostgresCrossTenantSuite runs the suite against Postgres with
// row-level security (make test-pg), as the application role.
func TestPostgresCrossTenantSuite(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	app := storetest.AppRoleDatabase(t, url)
	runCrossTenant(t, func(o *server.Options) {
		o.Store, o.DatabaseURL, o.IDs, o.FilesDir = "postgres", app, ids.V7{}, t.TempDir()
	})
}

func runCrossTenant(t *testing.T, change func(*server.Options)) {
	acme, beta := mtEnv(t, change)
	s := &ctSuite{t: t, acme: acme, beta: beta, obj: map[string]string{}, statuses: map[int]int{}}
	s.seed()
	restOps := s.rest()
	s.betaStillWorks()
	tools := s.mcpTools()
	s.webSockets()
	s.crossCredentials()
	s.intact()
	t.Logf("cross-tenant coverage: %d REST operations, %d MCP tools, WebSocket /ws/ui (people and agents) and /ws/host; beta's answers on acme objects: %v", restOps, tools, s.statuses)
}

// --- acme's data

func (s *ctSuite) seed() {
	t, a := s.t, s.acme
	owner := server.DevAdminToken
	must := func(st, want int, what string) {
		t.Helper()
		if st != want {
			t.Fatalf("seed %s: %d", what, st)
		}
	}
	var created struct {
		ID    string `json:"id"`
		Slug  string `json:"slug"`
		Token string `json:"token"`
		Actor struct {
			ID string `json:"id"`
		} `json:"actor"`
	}
	// A person and an API-key agent.
	must(a.do("POST", "/api/v1/actors", owner, map[string]any{"kind": "human", "handle": acmeRef + "user", "display_name": acmeData + " user"}, &created), http.StatusCreated, "person")
	s.obj["actor"] = created.Actor.ID
	must(a.do("POST", "/api/v1/agents", owner, map[string]any{"handle": acmeRef + "-bot", "display_name": acmeData + " bot", "api_key": true}, &created), http.StatusCreated, "agent")
	s.obj["agent"], s.acmeAgent = created.Actor.ID, created.Token
	// A channel with the agent in it, a message, a reply, a reaction, a file.
	must(a.do("POST", "/api/v1/channels", owner, map[string]any{"name": acmeRef + "-room", "description": acmeData + " room", "member_ids": []string{s.obj["agent"]}}, &created), http.StatusCreated, "channel")
	s.obj["channel"] = created.ID
	m, st := a.send(owner, s.obj["channel"], map[string]any{"body": acmeData + " message"})
	must(st, http.StatusCreated, "message")
	s.obj["message"] = m.ID
	r, st := a.send(owner, s.obj["channel"], map[string]any{"body": acmeData + " reply", "thread_root_id": m.ID})
	must(st, http.StatusCreated, "reply")
	s.obj["reply"] = r.ID
	must(a.do("PUT", "/api/v1/messages/"+m.ID+"/reactions/%F0%9F%91%8D", owner, nil, nil), http.StatusNoContent, "reaction")
	s.obj["file"] = s.upload(a, owner, s.obj["channel"], acmeData+" file")
	// Wiki, task, schedule, sidebar section, token, git mapping.
	must(a.do("POST", "/api/v1/wiki", owner, map[string]any{"title": acmeData + " page", "body": acmeData + " wiki body"}, &created), http.StatusCreated, "wiki")
	s.obj["wiki"] = created.Slug
	must(a.do("POST", "/api/v1/planner/tasks", owner, map[string]any{"title": acmeData + " task"}, &created), http.StatusCreated, "task")
	s.obj["task"] = created.ID
	must(a.do("POST", "/api/v1/schedules", owner, map[string]any{"channel_id": s.obj["channel"], "body": acmeData + " schedule",
		"at": a.clock.Now().Add(48 * time.Hour).Format(time.RFC3339)}, &created), http.StatusCreated, "schedule")
	s.obj["schedule"] = created.ID
	must(a.do("POST", "/api/v1/sidebar/sections", owner, map[string]any{"name": acmeData + " section"}, &created), http.StatusCreated, "section")
	s.obj["section"] = created.ID
	must(a.do("POST", "/api/v1/actors/"+s.obj["actor"]+"/tokens", owner, map[string]any{"label": acmeData}, &created), http.StatusCreated, "token")
	var toks struct{ Tokens []struct{ ID string } }
	a.do("GET", "/api/v1/actors/"+s.obj["actor"]+"/tokens", owner, nil, &toks)
	s.obj["token"] = toks.Tokens[0].ID
	must(a.do("POST", "/api/v1/git/mappings", owner, map[string]any{"repo": acmeRef + "/repo", "channel_id": s.obj["channel"]}, nil), http.StatusOK, "git mapping")
	// A machine.
	s.obj["host"], s.acmeHost = a.enroll(acmeRef + "-pc")
	// What only agents make: an error report, a lease, a decision, a note, a question.
	var ref struct{ ID string }
	must(a.do("POST", "/api/v1/agent/errors", s.acmeAgent, map[string]any{"channel_id": s.obj["channel"], "summary": acmeData + " error"}, &ref), http.StatusCreated, "error")
	s.obj["error"] = ref.ID
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"lease_acquire", map[string]any{"resource": acmeRef + "-res", "note": acmeData + " lease"}},
		{"decision_add", map[string]any{"text": acmeData + " decision", "rationale": acmeData, "thread": m.ID}},
		{"note_set", map[string]any{"text": acmeData + " note", "thread": m.ID}},
		{"ask", map[string]any{"question": acmeData + " question", "thread": m.ID, "channel": "#" + acmeRef + "-room"}},
	} {
		if out, isErr := a.mcpOn(s.acmeAgent, c.tool, c.args); isErr {
			t.Fatalf("seed %s: %s", c.tool, out)
		}
	}
	if err := a.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	var leases struct{ Leases []struct{ ID string } }
	a.do("GET", "/api/v1/leases", owner, nil, &leases)
	var qs struct{ Questions []struct{ ID string } }
	a.do("GET", "/api/v1/questions", owner, nil, &qs)
	if len(leases.Leases) == 0 || len(qs.Questions) == 0 {
		t.Fatalf("seed: lease %d, question %d", len(leases.Leases), len(qs.Questions))
	}
	s.obj["lease"], s.obj["question"] = leases.Leases[0].ID, qs.Questions[0].ID

	acmeWS := acmeWorkspace(t, a)
	s.seedShortLived(acmeWS)

	for _, id := range s.obj {
		if _, err := ids.Parse(id); err == nil {
			s.acmeIDs = append(s.acmeIDs, id)
		}
	}
	s.acmeIDs = append(s.acmeIDs, acmeWS.String())

	// beta: its owner, an agent and a machine of its own.
	b := s.beta
	s.betaWS = betaWorkspace(t, b)
	var me struct{ ID string }
	b.do("GET", "/api/v1/me", betaOwnerToken, nil, &me)
	s.betaOwner, _ = ids.Parse(me.ID)
	must(b.do("POST", "/api/v1/agents", betaOwnerToken, map[string]any{"handle": "beta-bot", "api_key": true}, &created), http.StatusCreated, "beta agent")
	s.betaAgent = created.Token
	var pc struct{ Code string }
	b.do("POST", "/api/v1/hosts/pairing-codes", betaOwnerToken, nil, &pc)
	var enrolled struct {
		Token string `json:"token"`
	}
	must(b.do("POST", "/api/v1/hosts/enroll", "", map[string]any{"code": pc.Code, "name": "beta-pc"}, &enrolled), http.StatusCreated, "beta host")
	s.betaHost = enrolled.Token
	b.reportReady(s.betaHost)
}

// seedShortLived adds the acme objects that take a long flow to make (an
// approval waiting for a person, a delegation grant, a calendar, a
// compaction, a file fetch, a folder grant, a preview) straight through
// their stores (round 40): before, the suite tried these routes only with
// ids that exist nowhere.
func (s *ctSuite) seedShortLived(ws ids.ID) {
	t, a := s.t, s.acme
	ctx := db.WithWorkspace(context.Background(), ws)
	gen, now := ids.V7{}, a.clock.Now()
	ch, msg, agent, host := mustID(s.obj["channel"]), mustID(s.obj["message"]), mustID(s.obj["agent"]), mustID(s.obj["host"])
	owner, err := a.app.Identity.Authenticate(context.Background(), server.DevAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	put := func(kind string, id ids.ID, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", kind, err)
		}
		s.obj[kind] = id.String()
	}
	id := gen.New()
	put("approval", id, a.app.Approval.Store.Create(ctx, approval.ApprovalRequest{ID: id, WorkspaceID: ws, AgentID: agent, ChannelID: ch,
		ToolName: "Bash", Input: acmeData + " command", State: approval.StatePending, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}))
	delID := gen.New()
	if err := a.app.Delegation.Store.Insert(ctx, delegation.Delegation{ID: delID, WorkspaceID: ws, MessageID: msg, ChannelID: ch,
		FromAgentID: agent, ToAgentID: mustID(s.obj["actor"]), QuestionID: mustID(s.obj["question"]), State: delegation.StateApproved,
		DecidedBy: &owner.ID, DecidedAt: &now, CreatedAt: now}); err != nil {
		t.Fatalf("seed delegation: %v", err)
	}
	id = gen.New()
	other := mustID(s.obj["actor"])
	ga, gb := agent, other
	if gb.String() < ga.String() {
		ga, gb = gb, ga
	}
	put("delegationGrant", id, a.app.Delegation.Store.InsertGrant(ctx, delegation.Grant{ID: id, WorkspaceID: ws, ChannelID: ch, AgentAID: ga, AgentBID: gb,
		DelegationID: delID, ApprovedBy: owner.ID, ApprovedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour)}))
	id = gen.New()
	put("calendar", id, a.app.Planner.Store.InsertCalendar(ctx, planner.Calendar{ID: id, WorkspaceID: ws, OwnerID: owner.ID, Name: acmeData + " calendar",
		URL: "https://calendar.example.com/" + acmeRef + ".ics", Colour: "blue", CreatedAt: now}))
	id = gen.New()
	put("compaction", id, a.app.Compaction.Store.InsertCompaction(ctx, compaction.Compaction{ID: id, WorkspaceID: ws, ChannelID: ch,
		Trigger: compaction.TriggerManual, RequestedBy: owner.ID, CompactorAgentID: agent, State: compaction.StatePending, CreatedAt: now}))
	id = gen.New()
	put("fileFetch", id, a.app.Files.Store.InsertFileFetchRequest(ctx, files.FileFetchRequest{ID: id, WorkspaceID: ws, MessageID: msg, ChannelID: ch,
		AgentID: agent, RequestedBy: owner.ID, Path: "/home/alice/" + acmeRef + ".txt", State: files.FileFetchPending, CreatedAt: now}))
	id = gen.New()
	put("grant", id, a.app.Hosts.Store.AddGrant(ctx, hosts.Grant{ID: id, WorkspaceID: ws, AgentID: agent, HostID: host,
		Path: "/home/alice/" + acmeRef, Mode: hosts.ModeRead, CreatedBy: owner.ID, CreatedAt: now}))
	id = gen.New()
	put("preview", id, a.app.Preview.Store.Create(ctx, preview.Preview{ID: id, WorkspaceID: ws, AgentID: agent, HostID: host, ChannelID: ch,
		Name: acmeRef + "-preview", ComposePath: "docker-compose.yaml", State: preview.StateRunning, URL: "http://192.0.2.1:41000",
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}))
}

func (s *ctSuite) upload(e *env, token, channel, content string) string {
	s.t.Helper()
	st, body := s.multipart(e, token, "/api/v1/channels/"+channel+"/files", content)
	if st != http.StatusCreated {
		s.t.Fatalf("upload: %d %s", st, body)
	}
	var f struct{ ID string }
	_ = json.Unmarshal([]byte(body), &f)
	return f.ID
}

func (s *ctSuite) multipart(e *env, token, path, content string) (int, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "notes.txt")
	_, _ = part.Write([]byte(content))
	_ = w.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+path, &buf)
	req.Host = e.host
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return send(s.t, req)
}

func send(t *testing.T, req *http.Request) (int, string) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// --- REST

// pathParam maps a path parameter to an acme object (or, for kinds acme
// has none of, an id that exists nowhere). A new parameter name must be
// added here.
func (s *ctSuite) pathParam(path, name string) (string, bool) {
	switch name {
	case "channelId":
		return s.obj["channel"], true
	case "messageId":
		return s.obj["message"], true
	case "fileId":
		return s.obj["file"], true
	case "actorId":
		return s.obj["actor"], true
	case "agentId":
		return s.obj["agent"], true
	case "hostId":
		return s.obj["host"], true
	case "tokenId":
		return s.obj["token"], true
	case "slug":
		return s.obj["wiki"], true
	case "sectionId":
		return s.obj["section"], true
	case "scheduleId":
		return s.obj["schedule"], true
	case "errorId":
		return s.obj["error"], true
	case "questionId":
		return s.obj["question"], true
	case "emoji":
		return "%F0%9F%91%8D", false
	case "owner":
		return acmeRef, false
	case "repo":
		return "repo", false
	case "version":
		return "1", false
	case "id":
		switch {
		case strings.HasPrefix(path, "/planner/tasks/"):
			return s.obj["task"], true
		case strings.HasPrefix(path, "/agents/"):
			return s.obj["agent"], true
		case strings.HasPrefix(path, "/leases/"):
			return s.obj["lease"], true
		case strings.HasPrefix(path, "/file-fetch/"), strings.HasPrefix(path, "/agent/file-fetch/"):
			return s.obj["fileFetch"], true
		case strings.HasPrefix(path, "/compactions/"), strings.HasPrefix(path, "/agent/compactions/"):
			return s.obj["compaction"], true
		case strings.HasPrefix(path, "/delegation-grants/"):
			return s.obj["delegationGrant"], true
		case strings.HasPrefix(path, "/planner/calendars/"):
			return s.obj["calendar"], true
		}
		s.t.Fatalf("cross-tenant suite: no acme object for {id} in %s; map it in pathParam", path)
		return "", false
	case "approvalId":
		return s.obj["approval"], true
	case "grantId":
		return s.obj["grant"], true
	case "previewId":
		return s.obj["preview"], true
	case "requestId":
		// Relay requests live in memory for seconds: acme has none; an id
		// that exists nowhere still has to be refused cleanly.
		return ids.V7{}.New().String(), false
	}
	s.t.Fatalf("cross-tenant suite: no acme object for path parameter %q (%s); map it in pathParam", name, path)
	return "", false
}

// okOnAcmeObject lists operations that may answer 2xx with an acme object
// in the path, and why that is not a leak. Each is still checked for
// leaked data, and acme's state is checked at the end.
var okOnAcmeObject = map[string]string{}

var paramRe = regexp.MustCompile(`\{(\w+)\}`)

func (s *ctSuite) rest() int {
	t := s.t
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type op struct {
		method, path string
		o            *openapi3.Operation
	}
	var ops []op
	for path, item := range doc.Paths.Map() {
		for method, o := range item.Operations() {
			ops = append(ops, op{method, path, o})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].o.OperationID < ops[j].o.OperationID })
	// Destructive operations last, so the others still find beta's own
	// credentials working.
	last := func(o op) bool {
		return strings.Contains(o.path, "logout") || strings.Contains(o.path, "emergency") || o.path == "/me/password" ||
			strings.HasPrefix(o.path, "/actors/{actorId}") && o.method == "DELETE"
	}
	sort.SliceStable(ops, func(i, j int) bool { return !last(ops[i]) && last(ops[j]) })

	for _, o := range ops {
		targetsAcme := false
		p := paramRe.ReplaceAllStringFunc(o.path, func(m string) string {
			v, acme := s.pathParam(o.path, m[1:len(m)-1])
			targetsAcme = targetsAcme || acme
			return v
		})
		path := "/api/v1" + p + s.query(o.o)
		token := s.credential(o.path)
		public := o.o.Security != nil && len(*o.o.Security) == 0
		body, ctype := s.body(o.o)

		// (a) On beta's own subdomain, with beta's credentials.
		st, resp := s.call(s.beta, o.method, path, token, body, ctype)
		s.checkLeak(o.o.OperationID, path, body, resp)
		if targetsAcme {
			s.statuses[st]++
		}
		if targetsAcme && st >= 200 && st < 300 {
			if why, ok := okOnAcmeObject[o.o.OperationID]; !ok {
				t.Errorf("%s %s (%s): beta got %d on an acme object: %.200s", o.method, o.path, o.o.OperationID, st, resp)
			} else {
				t.Logf("%s: 2xx on an acme object allowed: %s", o.o.OperationID, why)
			}
		}
		// (b) On acme's subdomain with a fresh beta credential: refused
		// before anything runs (the credential is revoked as leaked).
		if public || o.path == "/hooks/gitea" {
			continue
		}
		st, resp = s.call(s.acme, o.method, path, s.freshBetaToken(), body, ctype)
		if st != http.StatusUnauthorized {
			t.Errorf("%s %s (%s) on acme's subdomain with beta's token: %d, want 401", o.method, o.path, o.o.OperationID, st)
		}
		s.checkLeak(o.o.OperationID+" on acme", path, body, resp)
	}
	return len(ops)
}

// credential picks beta's credential for an operation: agents call the
// /agent/ routes, guards the /host/ routes, people the rest.
func (s *ctSuite) credential(path string) string {
	switch {
	case strings.HasPrefix(path, "/agent/") || path == "/agent":
		return s.betaAgent
	case strings.HasPrefix(path, "/host/"):
		return s.betaHost
	}
	return betaOwnerToken
}

// freshBetaToken mints a token for beta's owner (each cross-tenant use
// revokes the one used).
func (s *ctSuite) freshBetaToken() string {
	secret, err := identity.NewSecret()
	if err != nil {
		s.t.Fatal(err)
	}
	ctx := db.WithWorkspace(context.Background(), s.betaWS)
	tok := identity.Token{ID: ids.V7{}.New(), ActorID: s.betaOwner, Hash: identity.HashSecret(secret), Label: "suite",
		Prefix: identity.TokenPrefix(secret), CreatedAt: s.beta.clock.Now()}
	if err := s.beta.app.Identity.Store.CreateToken(ctx, tok); err != nil {
		s.t.Fatal(err)
	}
	return secret
}

func (s *ctSuite) call(e *env, method, path, token string, body []byte, ctype string) (int, string) {
	req, _ := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(body))
	req.Host = e.host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	return send(s.t, req)
}

// checkLeak fails when a response carries acme content, or an acme id that
// beta did not send itself.
func (s *ctSuite) checkLeak(what, path string, body []byte, resp string) {
	s.t.Helper()
	if strings.Contains(strings.ToLower(resp), acmeData) {
		s.t.Errorf("%s: acme data in the response: %.300s", what, resp)
	}
	for _, id := range s.acmeIDs {
		if strings.Contains(resp, id) && !strings.Contains(path, id) && !bytes.Contains(body, []byte(id)) {
			s.t.Errorf("%s: an acme id (%s) beta did not send is in the response: %.300s", what, id, resp)
		}
	}
}

// query fills the required query parameters, and id-like optional ones
// with acme's objects.
func (s *ctSuite) query(o *openapi3.Operation) string {
	v := url.Values{}
	for _, pr := range o.Parameters {
		p := pr.Value
		if p == nil || p.In != "query" {
			continue
		}
		val, ok := s.value(p.Name, p.Schema)
		if !ok && !p.Required {
			continue
		}
		v.Set(p.Name, fmt.Sprint(val))
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// body builds a request body from the operation's schema: the required
// fields, and every field that names an object filled with acme's.
func (s *ctSuite) body(o *openapi3.Operation) ([]byte, string) {
	if o.RequestBody == nil || o.RequestBody.Value == nil {
		return nil, ""
	}
	c := o.RequestBody.Value.Content
	if mt := c.Get("application/json"); mt != nil {
		v := s.object(mt.Schema)
		b, _ := json.Marshal(v)
		return b, "application/json"
	}
	if c.Get("multipart/form-data") != nil {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, _ := w.CreateFormFile("file", "beta.txt")
		_, _ = part.Write([]byte("beta upload"))
		_ = w.Close()
		return buf.Bytes(), w.FormDataContentType()
	}
	return []byte("beta bytes"), "application/octet-stream"
}

func (s *ctSuite) object(ref *openapi3.SchemaRef) map[string]any {
	out := map[string]any{}
	if ref == nil || ref.Value == nil {
		return out
	}
	sc := ref.Value
	for _, all := range sc.AllOf {
		for k, v := range s.object(all) {
			out[k] = v
		}
	}
	for name, prop := range sc.Properties {
		val, targets := s.value(name, prop)
		if targets || slices.Contains(sc.Required, name) {
			out[name] = val
		}
	}
	return out
}

// value is a plausible value for a field; targets is true when it names an
// acme object.
func (s *ctSuite) value(name string, ref *openapi3.SchemaRef) (any, bool) {
	byName := map[string]string{
		"channel_id": "channel", "thread_root_id": "message", "message_id": "message", "answers_message_id": "message",
		"reply_message_id": "message", "before_id": "message", "after_id": "message", "actor_id": "actor", "owner_id": "actor",
		"partner_id": "agent", "compactor_agent_id": "agent", "host_id": "host", "file_id": "file", "error_id": "error",
		"section_id": "section",
	}
	listByName := map[string]string{
		"actor_ids": "actor", "member_ids": "actor", "channel_ids": "channel", "file_ids": "file", "section_ids": "section",
		"task_agent_ids": "agent", "cal_agent_ids": "agent", "ids": "message",
	}
	if k, ok := byName[name]; ok {
		return s.obj[k], true
	}
	if k, ok := listByName[name]; ok {
		return []string{s.obj[k]}, true
	}
	if name == "repo" {
		return acmeRef + "/repo", true
	}
	if ref == nil || ref.Value == nil {
		return "x", false
	}
	sc := ref.Value
	if len(sc.Enum) > 0 {
		return sc.Enum[0], false
	}
	switch {
	case sc.Type.Is("object"):
		return s.object(ref), false
	case sc.Type.Is("array"):
		if sc.MinItems > 0 && sc.Items != nil {
			v, _ := s.value(name, sc.Items)
			return []any{v}, false
		}
		return []any{}, false
	case sc.Type.Is("integer"), sc.Type.Is("number"):
		if sc.Min != nil {
			return *sc.Min, false
		}
		return 1, false
	case sc.Type.Is("boolean"):
		return false, false
	}
	switch sc.Format {
	case "uuid":
		return ids.V7{}.New().String(), false
	case "date-time":
		return s.beta.clock.Now().Add(time.Hour).Format(time.RFC3339), false
	case "date":
		return s.beta.clock.Now().Format("2006-01-02"), false
	}
	v := "beta"
	for uint64(len(v)) < sc.MinLength {
		v += "-beta"
	}
	return v, false
}

// --- MCP

func (s *ctSuite) mcpTools() int {
	t := s.t
	ctx := context.Background()
	sess, err := fakeagent.Connect(ctx, s.beta.srv.URL+"/mcp", s.betaAgent, hostHeader{s.beta.host})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	list, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		args := s.toolArgs(tool)
		raw, _ := json.Marshal(args)
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: args})
		if err != nil {
			continue // a protocol error carries no data
		}
		var out strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				out.WriteString(tc.Text)
			}
		}
		if res.StructuredContent != nil {
			b, _ := json.Marshal(res.StructuredContent)
			out.Write(b)
		}
		s.checkLeak("MCP "+tool.Name, "", raw, out.String())
	}
	return len(list.Tools)
}

// toolArgs fills a tool's arguments: the required ones, and every one that
// names an object pointed at acme's.
func (s *ctSuite) toolArgs(tool *mcp.Tool) map[string]any {
	raw, _ := json.Marshal(tool.InputSchema)
	var sc struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(raw, &sc)
	refs := map[string]any{
		"thread": s.obj["message"], "message": s.obj["message"], "message_id": s.obj["message"], "root": s.obj["message"],
		"channel": "#" + acmeRef + "-room", "channel_id": s.obj["channel"], "to": "@" + acmeRef + "user",
		"question": s.obj["question"], "resource": acmeRef + "-res", "file": s.obj["file"], "file_id": s.obj["file"],
		"slug": s.obj["wiki"], "page": s.obj["wiki"], "agent": "@" + acmeRef + "-bot", "handle": acmeRef + "-bot",
		"id": s.obj["message"], "ids": []string{s.obj["message"]}, "approval": ids.V7{}.New().String(),
	}
	args := map[string]any{}
	for name, p := range sc.Properties {
		if v, ok := refs[name]; ok {
			args[name] = v
			continue
		}
		if !slices.Contains(sc.Required, name) {
			continue
		}
		switch fmt.Sprint(p.Type) {
		case "integer", "number":
			args[name] = 1
		case "boolean":
			args[name] = false
		case "array":
			args[name] = []string{"beta"}
		default:
			args[name] = "beta"
		}
	}
	return args
}

// hostHeader sends requests with a Host header (a subdomain).
type hostHeader struct{ host string }

func (h hostHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = h.host
	return http.DefaultTransport.RoundTrip(r)
}

// --- WebSockets

func (s *ctSuite) webSockets() {
	t := s.t
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dial := func(e *env, path, token string) (*websocket.Conn, map[string]any) {
		ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+path, &websocket.DialOptions{Host: e.host})
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
		if err != nil {
			return nil, nil
		}
		_ = wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": token})
		var first map[string]any
		if err := wsjson.Read(ctx, ws, &first); err != nil {
			_ = ws.CloseNow()
			return nil, nil
		}
		return ws, first
	}
	// beta's person and agent listen on beta's subdomain.
	var listeners []*websocket.Conn
	for _, tok := range []string{betaOwnerToken, s.betaAgent} {
		ws, first := dial(s.beta, "/ws/ui", tok)
		if ws == nil || first["type"] != "welcome" {
			t.Fatalf("beta's WebSocket: %v", first)
		}
		listeners = append(listeners, ws)
	}
	// acme is busy: a message mentioning everyone, a reply, a reaction.
	m, _ := s.acme.send(server.DevAdminToken, s.obj["channel"], map[string]any{"body": acmeData + " live @" + acmeRef + "-bot"})
	s.acme.send(server.DevAdminToken, s.obj["channel"], map[string]any{"body": acmeData + " live reply", "thread_root_id": m.ID})
	if err := s.acme.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i, ws := range listeners {
		rctx, rcancel := context.WithTimeout(ctx, 500*time.Millisecond)
		for {
			var f map[string]any
			if err := wsjson.Read(rctx, ws, &f); err != nil {
				break
			}
			b, _ := json.Marshal(f)
			s.checkLeak(fmt.Sprintf("WebSocket listener %d", i), "", nil, string(b))
		}
		rcancel()
		_ = ws.CloseNow()
	}
	s.dial = dial
}

// crossCredentials runs last: beta's agent and guard credentials on acme's
// subdomain are refused (and revoked as leaked).
func (s *ctSuite) crossCredentials() {
	t := s.t
	for name, c := range map[string]struct{ path, token string }{
		"person": {"/ws/ui", s.freshBetaToken()}, "agent": {"/ws/ui", s.betaAgent}, "guard": {"/ws/host", s.betaHost},
	} {
		ws, first := s.dial(s.acme, c.path, c.token)
		if ws != nil {
			_ = ws.CloseNow()
		}
		if first["type"] == "welcome" {
			t.Errorf("beta's %s was welcomed on acme's %s", name, c.path)
		}
	}
	ctx := context.Background()
	if sess, err := fakeagent.Connect(ctx, s.acme.srv.URL+"/mcp", s.freshBetaToken(), hostHeader{s.acme.host}); err == nil {
		_ = sess.Close()
		t.Error("a beta credential connected to acme's MCP endpoint")
	}
}

// betaStillWorks makes sure beta's own credentials survived the REST pass,
// so that the refusals above were about acme's objects, not a dead token.
func (s *ctSuite) betaStillWorks() {
	for name, c := range map[string]struct{ path, token string }{
		"owner": {"/api/v1/me", betaOwnerToken}, "agent": {"/api/v1/me", s.betaAgent}, "host": {"/api/v1/host/config", s.betaHost},
	} {
		if st, body := s.beta.raw("GET", c.path, c.token, nil); st != http.StatusOK {
			s.t.Fatalf("beta's %s credential stopped working during the suite: %d %s", name, st, body)
		}
	}
}

// --- acme afterwards

func (s *ctSuite) intact() {
	t, a, owner := s.t, s.acme, server.DevAdminToken
	if err := a.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	check := func(path string, want ...string) {
		t.Helper()
		st, body := a.raw("GET", path, owner, nil)
		if st != http.StatusOK {
			t.Errorf("acme %s after the suite: %d %.200s", path, st, body)
			return
		}
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("acme %s after the suite lacks %q: %.300s", path, w, body)
			}
		}
	}
	check("/api/v1/me")
	check("/api/v1/channels/"+s.obj["channel"], acmeData+" room")
	check("/api/v1/messages/"+s.obj["message"]+"/thread", acmeData+" message", acmeData+" reply", "👍")
	check("/api/v1/wiki/"+s.obj["wiki"], acmeData+" wiki body")
	check("/api/v1/planner/tasks", acmeData+" task")
	check("/api/v1/schedules", acmeData+" schedule")
	check("/api/v1/sidebar/sections", acmeData+" section")
	check("/api/v1/actors/"+s.obj["actor"]+"/tokens", s.obj["token"])
	check("/api/v1/agents/"+s.obj["agent"]+"/overview", acmeRef+"-bot")
	check("/api/v1/hosts", acmeRef+"-pc", `"approved"`)
	check("/api/v1/files/"+s.obj["file"]+"/content", acmeData+" file")
	check("/api/v1/leases", acmeData+" lease")
	check("/api/v1/questions", acmeData+" question")
	check("/api/v1/git/mappings", acmeRef+"/repo")
	if st, _ := a.raw("GET", "/api/v1/me", s.acmeAgent, nil); st != http.StatusOK {
		t.Errorf("acme's agent token after the suite: %d", st)
	}
	if st, _ := a.raw("GET", "/api/v1/host/config", s.acmeHost, nil); st != http.StatusOK {
		t.Errorf("acme's host token after the suite: %d", st)
	}
	var ws struct {
		EmergencyStop struct {
			Active bool `json:"active"`
		} `json:"emergency_stop"`
	}
	a.do("GET", "/api/v1/admin/status", owner, nil, &ws)
	if ws.EmergencyStop.Active {
		t.Error("beta's emergency stop reached acme")
	}
}

// mcpOn is mcpCall on the env's subdomain.
func (e *env) mcpOn(token, tool string, args map[string]any) (string, bool) {
	e.t.Helper()
	ctx := context.Background()
	sess, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", token, hostHeader{e.host})
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		e.t.Fatal(err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}
