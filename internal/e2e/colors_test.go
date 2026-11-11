package e2e

import (
	"net/http"
	"testing"

	"colonyhall/internal/server"
)

func (e *env) effectiveColor(handle string) (own, effective string) {
	e.t.Helper()
	var list struct {
		Actors []struct {
			Handle         string
			Color          string
			EffectiveColor string `json:"effective_color"`
		}
	}
	e.do("GET", "/api/v1/actors", server.DevAdminToken, nil, &list)
	for _, a := range list.Actors {
		if a.Handle == handle {
			return a.Color, a.EffectiveColor
		}
	}
	e.t.Fatalf("@%s not listed", handle)
	return "", ""
}

func TestHostAndAgentColors(t *testing.T) {
	e := newEnv(t)
	first, _ := e.enroll("pc")
	second, _ := e.enroll("homelab")
	var hosts struct {
		Hosts []struct{ ID, Name, Color string }
	}
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	byID := map[string]string{}
	for _, h := range hosts.Hosts {
		byID[h.ID] = h.Color
	}
	// New machines get the palette in order.
	if byID[first] != "blue" || byID[second] != "green" {
		t.Fatalf("default colours: %v", byID)
	}

	// Only human admins recolour a machine, and only to a palette hue.
	_, memberToken := e.createActor("human", "anna", "member")
	for _, tok := range []string{memberToken, server.DevAgentToken} {
		if s := e.do("PATCH", "/api/v1/hosts/"+first, tok, map[string]any{"color": "teal"}, nil); s != http.StatusForbidden {
			t.Fatalf("non-admin recolours a host: %d", s)
		}
	}
	if s := e.do("PATCH", "/api/v1/hosts/"+first, server.DevAdminToken, map[string]any{"color": "#ff0000"}, nil); s != http.StatusBadRequest {
		t.Fatalf("hex colour: %d", s)
	}
	var updated struct{ Name, Color string }
	if s := e.do("PATCH", "/api/v1/hosts/"+first, server.DevAdminToken, map[string]any{"color": "teal", "name": "pc-dev"}, &updated); s != http.StatusOK ||
		updated.Color != "teal" || updated.Name != "pc-dev" {
		t.Fatalf("recolour: %d %+v", s, updated)
	}

	// An agent shows its machine's colour until it has its own.
	agent := e.agentID()
	e.configure(agent, map[string]any{"host_id": first, "client": "claude-code", "model": "haiku", "root_dir": "/app"})
	if own, eff := e.effectiveColor("fake-agent"); own != "" || eff != "teal" {
		t.Fatalf("machine colour: %q %q", own, eff)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+agent+"/profile", memberToken, map[string]any{"color": "pink"}, nil); s != http.StatusForbidden {
		t.Fatalf("member sets an agent colour: %d", s)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+agent+"/profile", server.DevAdminToken, map[string]any{"color": "pink"}, nil); s != http.StatusOK {
		t.Fatalf("admin sets an agent colour: %d", s)
	}
	if own, eff := e.effectiveColor("fake-agent"); own != "pink" || eff != "pink" {
		t.Fatalf("own colour: %q %q", own, eff)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+agent+"/profile", server.DevAdminToken, map[string]any{"color": ""}, nil); s != http.StatusOK {
		t.Fatalf("clear: %d", s)
	}
	if own, eff := e.effectiveColor("fake-agent"); own != "" || eff != "teal" {
		t.Fatalf("cleared: %q %q", own, eff)
	}
	// Humans have no colour this round.
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me)
	if s := e.do("PATCH", "/api/v1/actors/"+me.ID+"/profile", server.DevAdminToken, map[string]any{"color": "red"}, nil); s != http.StatusBadRequest {
		t.Fatalf("human colour: %d", s)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+agent+"/profile", server.DevAdminToken, map[string]any{"color": "accent"}, nil); s != http.StatusBadRequest {
		t.Fatalf("unknown hue: %d", s)
	}
}
