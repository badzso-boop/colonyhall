package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

func TestSystemStatus(t *testing.T) {
	e := newEnv(t)
	_, memberToken := e.createActor("human", "anna", "member")

	var member apispec.AdminStatus
	if st := e.do(http.MethodGet, "/api/v1/admin/status", memberToken, nil, &member); st != http.StatusOK {
		t.Fatalf("member status: %d", st)
	}
	if member.System != nil {
		t.Fatal("a member must only see the emergency stop")
	}

	_, agentAdmin := e.createActor("agent", "boss-bot", "admin")
	var bot apispec.AdminStatus
	if st := e.do(http.MethodGet, "/api/v1/admin/status", agentAdmin, nil, &bot); st != http.StatusOK || bot.System != nil {
		t.Fatalf("an agent admin gets the banner only: %d %+v", st, bot.System)
	}

	var pc struct{ Code string }
	e.do(http.MethodPost, "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(t.TempDir(), "host.token")
	if err := guard.Enroll(t.Context(), e.srv.URL, pc.Code, "pc", tokenFile); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if st := e.do(http.MethodPost, "/api/v1/host/report", strings.TrimSpace(string(raw)), map[string]any{"guard_version": "v1"}, nil); st != http.StatusNoContent {
		t.Fatalf("report: %d", st)
	}
	var seen apispec.AdminStatus
	e.do(http.MethodGet, "/api/v1/admin/status", server.DevAdminToken, nil, &seen)
	if len(seen.System.Guards) != 1 || !seen.System.Guards[0].Online || *seen.System.Guards[0].GuardVersion != "v1" {
		t.Fatalf("a guard that just reported is online: %+v", seen.System.Guards)
	}

	// Stopped consumers fall behind: the lag is measured, then disappears.
	e.send(server.DevAdminToken, e.generalID(), map[string]any{"body": "hello"})
	e.clock.Advance(2 * time.Minute)
	var before apispec.AdminStatus
	e.do(http.MethodGet, "/api/v1/admin/status", server.DevAdminToken, nil, &before)
	if before.System == nil || !before.System.Database.Ok || before.System.EventHead == 0 || before.System.UptimeSeconds < 120 {
		t.Fatalf("system: %+v", before.System)
	}
	lagging := false
	for _, c := range before.System.Consumers {
		lagging = lagging || (c.Lag > 0 && c.LagSeconds >= 120)
	}
	if !lagging {
		t.Fatalf("no consumer reported lag: %+v", before.System.Consumers)
	}
	if len(before.System.Guards) != 1 || before.System.Guards[0].Online {
		t.Fatalf("a guard silent for two minutes is offline: %+v", before.System.Guards)
	}
	if err := e.app.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var after apispec.AdminStatus
	e.do(http.MethodGet, "/api/v1/admin/status", server.DevAdminToken, nil, &after)
	for _, c := range after.System.Consumers {
		if c.Lag != 0 {
			t.Fatalf("still lagging after drain: %+v", c)
		}
	}
	if after.System.GiteaConfigured {
		t.Fatal("no Gitea in this environment")
	}
	fields := ""
	var text map[string]any
	e.do(http.MethodGet, "/api/v1/admin/status", server.DevAdminToken, nil, &text)
	for k := range text["system"].(map[string]any) {
		fields += k + " "
	}
	for _, secret := range []string{"token", "secret", "password", "url"} {
		if strings.Contains(fields, secret) {
			t.Fatalf("system status field looks like a secret: %s", fields)
		}
	}
}
