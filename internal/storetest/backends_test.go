package storetest

import (
	"context"
	"os"
	"testing"

	evmem "colonyhall/internal/eventlog/memory"
	evpg "colonyhall/internal/eventlog/postgres"
	convmem "colonyhall/internal/modules/conversation/memory"
	convpg "colonyhall/internal/modules/conversation/postgres"
	delmem "colonyhall/internal/modules/delivery/memory"
	delpg "colonyhall/internal/modules/delivery/postgres"
	idmem "colonyhall/internal/modules/identity/memory"
	idpg "colonyhall/internal/modules/identity/postgres"
	"colonyhall/internal/platform/db"
	"colonyhall/migrations"
)

func TestMemory(t *testing.T) {
	Run(t, func(t *testing.T) Backend {
		return Backend{Tx: &db.Memory{}, Events: evmem.New(), Identity: idmem.New(),
			Conversation: convmem.New(), Delivery: delmem.New()}
	})
}

// TestPostgres runs the suite against a real database. It needs
// COLONY_TEST_DATABASE_URL (see `make test-pg`) and wipes that database.
func TestPostgres(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pg, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Pool.Close)
	if err := migrations.Up(ctx, pg.Pool); err != nil {
		t.Fatal(err)
	}
	Run(t, func(t *testing.T) Backend {
		_, err := pg.Pool.Exec(ctx, `TRUNCATE delivery, consumer_offset, event, message, channel_member,
			channel, session, credential, api_token, actor, workspace RESTART IDENTITY CASCADE`)
		if err != nil {
			t.Fatal(err)
		}
		return Backend{Tx: pg, Events: &evpg.Store{DB: pg}, Identity: &idpg.Store{DB: pg},
			Conversation: &convpg.Store{DB: pg}, Delivery: &delpg.Store{DB: pg}}
	})
}
