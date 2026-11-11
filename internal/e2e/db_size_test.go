package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// Round 50: the memory store has no database, so the size fields are absent.
func TestSystemStatusDatabaseSizeMemory(t *testing.T) {
	e := newEnv(t)
	e.app.Maintain(t.Context()) // sampling without a database is a no-op
	var st apispec.AdminStatus
	if s := e.do(http.MethodGet, "/api/v1/admin/status", server.DevAdminToken, nil, &st); s != http.StatusOK || st.System == nil {
		t.Fatalf("status: %d", s)
	}
	db := st.System.Database
	if db.SizeBytes != nil || db.Tables != nil || db.Growth != nil {
		t.Fatalf("memory store reports a database size: %+v", db)
	}
}

// TestPostgresSystemStatusDatabaseSize checks the size, the largest tables
// and the daily samples against a real database; it runs with `make test-pg`
// (COLONY_TEST_DATABASE_URL), whose database it shares with the storetest.
func TestPostgresSystemStatusDatabaseSize(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	clk := clock.NewManual(time.Now().UTC())
	app, err := server.New(ctx, server.Options{Store: "postgres", DatabaseURL: url, Clock: clk, IDs: ids.V7{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	e := &env{t: t, app: app, srv: srv, router: specRouter(t, srv.URL), clock: clk}

	app.Maintain(ctx)
	clk.Advance(24 * time.Hour)
	app.Maintain(ctx)
	app.Maintain(ctx) // the same day again: no second sample

	var st apispec.AdminStatus
	if s := e.do(http.MethodGet, "/api/v1/admin/status", server.DevAdminToken, nil, &st); s != http.StatusOK || st.System == nil {
		t.Fatalf("status: %d", s)
	}
	db := st.System.Database
	if db.SizeBytes == nil || *db.SizeBytes <= 0 || db.Tables == nil || db.Growth == nil {
		t.Fatalf("database size missing: %+v", db)
	}
	if n := len(*db.Tables); n == 0 || n > 10 {
		t.Fatalf("tables: %d", n)
	}
	if n := len(*db.Growth); n < 2 {
		t.Fatalf("growth samples: %+v", *db.Growth)
	}
	g := *db.Growth
	last := g[len(g)-1].Date.Time
	if !last.Equal(g[len(g)-2].Date.AddDate(0, 0, 1)) {
		t.Fatalf("one sample a day: %+v", g)
	}

	// A member never sees it.
	_, memberToken := e.createActor("human", fmt.Sprintf("size-member-%d", clk.Now().UnixNano()%1_000_000), "member") // the database outlives the test
	var member apispec.AdminStatus
	if s := e.do(http.MethodGet, "/api/v1/admin/status", memberToken, nil, &member); s != http.StatusOK || member.System != nil {
		t.Fatalf("a member sees the system status: %d", s)
	}
}
