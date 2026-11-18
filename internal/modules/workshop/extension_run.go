// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

var templateRe = regexp.MustCompile(`\{\{\s*input\.([A-Za-z0-9_]+)\s*\}\}`)

// inputValue returns input[key] (nil if absent).
func inputValue(input map[string]any, key string) any { return input[key] }

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// fill replaces {{input.x}} in a string; escape is applied to every value.
func fill(s string, input map[string]any, escape func(string) string) string {
	return templateRe.ReplaceAllStringFunc(s, func(m string) string {
		key := templateRe.FindStringSubmatch(m)[1]
		return escape(asString(inputValue(input, key)))
	})
}

// fillJSON replaces templates in a JSON body: a string that is exactly
// one template becomes the input's value with its type; other strings get
// the values as text.
func fillJSON(v any, input map[string]any) any {
	switch x := v.(type) {
	case string:
		if m := templateRe.FindStringSubmatch(x); m != nil && m[0] == strings.TrimSpace(x) {
			return inputValue(input, m[1])
		}
		return fill(x, input, func(s string) string { return s })
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = fillJSON(e, input)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = fillJSON(e, input)
		}
		return out
	}
	return v
}

// connectorEndpoint returns a connector of the manifest with its
// authentication headers.
func (s *Service) connectorEndpoint(ctx context.Context, workspaceID ids.ID, m Manifest, slug string) (Connector, Endpoint, error) {
	listed := false
	for _, c := range m.Connectors {
		listed = listed || c == slug
	}
	if !listed {
		return Connector{}, Endpoint{}, fmt.Errorf("connector %q is not in the extension's manifest", slug)
	}
	c, err := s.connectorBySlug(ctx, workspaceID, slug)
	if err != nil {
		return Connector{}, Endpoint{}, fmt.Errorf("connector %q no longer exists", slug)
	}
	ep, err := s.endpoint(ctx, c)
	return c, ep, err
}

// runHTTPTool runs a declarative tool: the request from its template, the
// answer through its output map.
func (s *Service) runHTTPTool(ctx context.Context, rc runCtx) (Result, error) {
	r := rc.tool.Request
	if r == nil || s.Fetcher == nil {
		return Result{}, errors.New("the tool has no request")
	}
	var input map[string]any
	if len(rc.input) > 0 {
		if err := json.Unmarshal(rc.input, &input); err != nil {
			return Result{}, errors.New("the input must be a JSON object")
		}
	}
	c, ep, err := s.connectorEndpoint(ctx, rc.ext.WorkspaceID, rc.version.Manifest, r.Connector)
	if err != nil {
		return Result{}, err
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return Result{}, errors.New("the connector's address is invalid")
	}
	// An input may name a path of several segments (owner/repo): each
	// segment is escaped, and "." and ".." are refused, so it never leaves
	// the template's place (field test, 2026-11-18: golang/go became
	// golang%2Fgo).
	badSegment := false
	path := fill(r.Path, input, func(v string) string {
		parts := strings.Split(v, "/")
		for i, p := range parts {
			if p == "." || p == ".." || p == "" && len(parts) > 1 {
				badSegment = true
			}
			parts[i] = url.PathEscape(p)
		}
		return strings.Join(parts, "/")
	})
	if badSegment {
		return Result{}, errors.New("an input in the path may not contain empty, . or .. segments")
	}
	u := *base
	raw := strings.TrimRight(base.EscapedPath(), "/") + path
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		return Result{}, errors.New("invalid path")
	}
	u.Path, u.RawPath = unescaped, raw
	q := u.Query()
	for k, v := range r.Query {
		q.Set(k, fill(v, input, func(s string) string { return s }))
	}
	u.RawQuery = q.Encode()
	if u.Host != base.Host || u.Scheme != base.Scheme {
		return Result{}, errors.New("the request left the connector")
	}
	headers := map[string]string{"Accept": "application/json"}
	for k, v := range r.Headers {
		headers[k] = fill(v, input, func(s string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(s) })
	}
	var body []byte
	if len(r.Body) > 0 {
		var tmpl any
		if err := json.Unmarshal(r.Body, &tmpl); err != nil {
			return Result{}, err
		}
		body, _ = json.Marshal(fillJSON(tmpl, input))
		headers["Content-Type"] = "application/json"
	}
	res, err := s.Fetcher.Fetch(ctx, FetchRequest{Method: r.Method, URL: u.String(), Headers: headers, Auth: ep.Header, Body: body})
	if err != nil {
		return Result{}, err
	}
	text := string(res.Body)
	if rc.tool.Output != nil && len(rc.tool.Output.Pick) > 0 && strings.Contains(res.ContentType, "json") {
		var doc any
		if json.Unmarshal(res.Body, &doc) == nil {
			b, _ := json.Marshal(pick(doc, rc.tool.Output))
			text = string(b)
		}
	}
	if res.Status >= 400 {
		return Result{Text: fmt.Sprintf("HTTP %d: %s", res.Status, clip(text, 2000)), IsError: true}, nil
	}
	return Result{Text: text}, nil
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// path evaluates a simple JSON path: "a.b", "items[].key" (map over an
// array), "items[0].key".
func evalPath(v any, path string) any {
	if path == "" {
		return v
	}
	head, rest, _ := strings.Cut(path, ".")
	if strings.HasSuffix(head, "[]") {
		arr, _ := field(v, strings.TrimSuffix(head, "[]")).([]any)
		out := make([]any, 0, len(arr))
		for _, e := range arr {
			out = append(out, evalPath(e, rest))
		}
		return out
	}
	if i := strings.Index(head, "["); i > 0 && strings.HasSuffix(head, "]") {
		arr, _ := field(v, head[:i]).([]any)
		var n int
		if _, err := fmt.Sscanf(head[i+1:len(head)-1], "%d", &n); err != nil || n < 0 || n >= len(arr) {
			return nil
		}
		return evalPath(arr[n], rest)
	}
	return evalPath(field(v, head), rest)
}

func field(v any, name string) any {
	if name == "" {
		return v
	}
	m, _ := v.(map[string]any)
	return m[name]
}

// pick applies an output map: when every path maps over the same array
// ("issues[].key", "issues[].fields.summary") the answer is a list of
// objects; otherwise an object of the values.
func pick(doc any, o *OutputMap) any {
	max := o.MaxItems
	if max <= 0 || max > 200 {
		max = 50
	}
	prefix := ""
	same := true
	for _, p := range o.Pick {
		i := strings.Index(p, "[].")
		if i < 0 {
			same = false
			break
		}
		if prefix == "" {
			prefix = p[:i]
		} else if prefix != p[:i] {
			same = false
		}
	}
	if same && prefix != "" {
		arr, _ := evalPath(doc, prefix).([]any)
		if len(arr) > max {
			arr = arr[:max]
		}
		out := make([]map[string]any, 0, len(arr))
		for _, e := range arr {
			row := map[string]any{}
			for name, p := range o.Pick {
				row[name] = evalPath(e, strings.TrimPrefix(p, prefix+"[]."))
			}
			out = append(out, row)
		}
		return out
	}
	out := map[string]any{}
	for name, p := range o.Pick {
		v := evalPath(doc, p)
		if arr, ok := v.([]any); ok && len(arr) > max {
			v = arr[:max]
		}
		out[name] = v
	}
	return out
}

// host answers a js tool's host calls (docs/25 13.4): fetch, store,
// colony, call. Every one acts in the call's workspace.
func (s *Service) host(ctx context.Context, rc runCtx, kind string, payload json.RawMessage) (json.RawMessage, error) {
	// A remote runner's call arrives on the internal listener without a
	// scope: every host call acts in the extension's workspace.
	ctx = db.WithWorkspace(ctx, rc.ext.WorkspaceID)
	switch kind {
	case "fetch":
		return s.hostFetch(ctx, rc, payload)
	case "store":
		return s.hostStore(ctx, rc, payload)
	case "colony":
		var in struct {
			Tool  string          `json:"tool"`
			Input json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, errors.New("bad colony call")
		}
		if s.Core == nil || strings.Contains(in.Tool, "__") {
			return nil, fmt.Errorf("unknown Colonyhall tool %q", in.Tool)
		}
		// A core tool that changes something is a write: a tool declared
		// read may not use it (its call had no approval for that).
		if risk := CoreToolRisk(in.Tool); riskRank(risk) > riskRank(rc.tool.Risk) {
			return nil, fmt.Errorf("colony.%s changes something: declare this tool's risk as write", in.Tool)
		}
		text, err := s.Core(ctx, rc.caller, in.Tool, in.Input)
		if err != nil {
			return nil, err
		}
		return json.Marshal(text)
	case "call":
		return s.hostCall(ctx, rc, payload)
	}
	return nil, fmt.Errorf("unknown host call %q", kind)
}

type fetchIn struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
	Auth    string            `json:"auth"`
}

func (s *Service) hostFetch(ctx context.Context, rc runCtx, payload json.RawMessage) (json.RawMessage, error) {
	var in fetchIn
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, errors.New("bad fetch call")
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Host == "" {
		return nil, errors.New("fetch needs an absolute URL")
	}
	m := rc.version.Manifest
	req := FetchRequest{Method: strings.ToUpper(in.Method), URL: in.URL, Headers: map[string]string{}}
	if req.Method == "" {
		req.Method = "GET"
	}
	for k, v := range in.Headers {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "host") || strings.EqualFold(k, "cookie") {
			continue
		}
		req.Headers[k] = strings.NewReplacer("\r", "", "\n", "").Replace(v)
	}
	if len(in.Body) > 0 && string(in.Body) != "null" {
		var str string
		if json.Unmarshal(in.Body, &str) == nil {
			req.Body = []byte(str)
		} else {
			req.Body = in.Body
			if req.Headers["Content-Type"] == "" {
				req.Headers["Content-Type"] = "application/json"
			}
		}
	}
	if in.Auth != "" {
		// The connector's secret, put on by the server; only to the
		// connector's own origin.
		c, ep, err := s.connectorEndpoint(ctx, rc.ext.WorkspaceID, m, in.Auth)
		if err != nil {
			return nil, err
		}
		if origin(c.BaseURL) != origin(in.URL) {
			return nil, fmt.Errorf("auth %q works only on the connector's own address", in.Auth)
		}
		req.Auth = ep.Header
	} else {
		host := strings.ToLower(u.Hostname())
		ok := false
		for _, h := range m.Egress {
			ok = ok || h == host
		}
		if !ok {
			return nil, fmt.Errorf("%s is not among the extension's egress hosts", host)
		}
		req.Hosts = m.Egress
	}
	res, err := s.Fetcher.Fetch(ctx, req)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"status": res.Status, "content_type": res.ContentType, "body": string(res.Body)}
	if strings.Contains(res.ContentType, "json") {
		var j any
		if json.Unmarshal(res.Body, &j) == nil {
			out["json"] = j
		}
	}
	return json.Marshal(out)
}

type storeIn struct {
	Op         string          `json:"op"`
	Collection string          `json:"collection"`
	ID         string          `json:"id"`
	Data       json.RawMessage `json:"data"`
	After      string          `json:"after"`
	Limit      int             `json:"limit"`
}

func (s *Service) hostStore(ctx context.Context, rc runCtx, payload json.RawMessage) (json.RawMessage, error) {
	var in storeIn
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, errors.New("bad store call")
	}
	declared := false
	for _, c := range rc.version.Manifest.Store {
		declared = declared || c == in.Collection
	}
	if !declared {
		return nil, fmt.Errorf("collection %q is not in the extension's manifest", in.Collection)
	}
	ws, ext := rc.ext.WorkspaceID, rc.ext.ID
	if in.Op != "list" && (in.ID == "" || len(in.ID) > 128) {
		return nil, errors.New("a document id is 1–128 characters")
	}
	switch in.Op {
	case "get":
		d, err := s.Store.GetDoc(ctx, ws, ext, in.Collection, in.ID)
		if errors.Is(err, db.ErrNotFound) {
			return json.RawMessage("null"), nil
		}
		if err != nil {
			return nil, err
		}
		return d.Data, nil
	case "put":
		if len(in.Data) == 0 || !json.Valid(in.Data) {
			return nil, errors.New("put needs JSON data")
		}
		if len(in.Data) > MaxStoreDoc {
			return nil, ErrStoreFull
		}
		n, size, err := s.Store.StoreUsage(ctx, ws, ext)
		if err != nil {
			return nil, err
		}
		old := 0
		if d, err := s.Store.GetDoc(ctx, ws, ext, in.Collection, in.ID); err == nil {
			old = d.SizeBytes
		} else {
			n++
		}
		if n > MaxStoreDocs || size-old+len(in.Data) > MaxStoreBytes {
			return nil, ErrStoreFull
		}
		err = s.Store.PutDoc(ctx, StoreDoc{WorkspaceID: ws, ExtensionID: ext, Collection: in.Collection, DocID: in.ID,
			Data: in.Data, SizeBytes: len(in.Data), UpdatedAt: s.Clock.Now()})
		return json.RawMessage("true"), err
	case "delete":
		return json.RawMessage("true"), s.Store.DeleteDoc(ctx, ws, ext, in.Collection, in.ID)
	case "list":
		limit := in.Limit
		if limit <= 0 || limit > 100 {
			limit = 100
		}
		ds, err := s.Store.ListDocs(ctx, ws, ext, in.Collection, in.After, limit)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(ds))
		for i, d := range ds {
			out[i] = map[string]any{"id": d.DocID, "data": d.Data}
		}
		return json.Marshal(out)
	}
	return nil, fmt.Errorf("unknown store operation %q", in.Op)
}

// hostCall runs another tool of the same extension, or a tool of a
// connector of its manifest. The inner tool's risk may not exceed the
// outer one's: the approval the outer call had covers only that much.
func (s *Service) hostCall(ctx context.Context, rc runCtx, payload json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Tool  string          `json:"tool"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, errors.New("bad call")
	}
	if rc.depth >= 3 {
		return nil, errors.New("calls are nested too deep")
	}
	if slug, tool, ok := SplitGatewayName(in.Tool); ok {
		c, ep, err := s.connectorEndpoint(ctx, rc.ext.WorkspaceID, rc.version.Manifest, slug)
		if err != nil {
			return nil, err
		}
		info, ok := c.Tool(tool)
		if !ok {
			return nil, ErrUnknownTool
		}
		if riskRank(c.EffectiveRisk(info)) > riskRank(rc.tool.Risk) {
			return nil, fmt.Errorf("%s is riskier (%s) than this tool (%s): declare this tool's risk at least as %s", in.Tool,
				c.EffectiveRisk(info), rc.tool.Risk, c.EffectiveRisk(info))
		}
		res, err := s.Remote.CallTool(ctx, ep, tool, in.Input)
		if err != nil {
			return nil, err
		}
		if res.IsError {
			return nil, errors.New(clip(res.Text, 1000))
		}
		return json.Marshal(res.Text)
	}
	mt, ok := rc.version.Manifest.Tool(in.Tool)
	if !ok {
		return nil, ErrUnknownTool
	}
	if riskRank(mt.Risk) > riskRank(rc.tool.Risk) {
		return nil, fmt.Errorf("%s is riskier (%s) than this tool (%s)", in.Tool, mt.Risk, rc.tool.Risk)
	}
	inner := rc
	inner.tool, inner.input, inner.depth = mt, in.Input, rc.depth+1
	res, _, err := s.runTool(ctx, inner)
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, errors.New(clip(res.Text, 1000))
	}
	var v any
	if json.Unmarshal([]byte(res.Text), &v) == nil {
		return json.RawMessage(res.Text), nil
	}
	return json.Marshal(res.Text)
}
