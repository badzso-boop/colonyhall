package storetest

import (
	rulesmem "colonyhall/internal/modules/agentrules/memory"
	rulespg "colonyhall/internal/modules/agentrules/postgres"
	decisionsmem "colonyhall/internal/modules/decision/memory"
	decisionspg "colonyhall/internal/modules/decision/postgres"
	leasesmem "colonyhall/internal/modules/lease/memory"
	leasespg "colonyhall/internal/modules/lease/postgres"
	notesmem "colonyhall/internal/modules/notes/memory"
	notespg "colonyhall/internal/modules/notes/postgres"
	previewmem "colonyhall/internal/modules/preview/memory"
	previewpg "colonyhall/internal/modules/preview/postgres"
	wikimem "colonyhall/internal/modules/wiki/memory"
	wikipg "colonyhall/internal/modules/wiki/postgres"
	"context"
	"os"
	"testing"

	evmem "colonyhall/internal/eventlog/memory"
	evpg "colonyhall/internal/eventlog/postgres"
	appmem "colonyhall/internal/modules/approval/memory"
	apppg "colonyhall/internal/modules/approval/postgres"
	audmem "colonyhall/internal/modules/audit/memory"
	audpg "colonyhall/internal/modules/audit/postgres"
	convmem "colonyhall/internal/modules/conversation/memory"
	convpg "colonyhall/internal/modules/conversation/postgres"
	delmem "colonyhall/internal/modules/delivery/memory"
	delpg "colonyhall/internal/modules/delivery/postgres"
	filesmem "colonyhall/internal/modules/files/memory"
	filespg "colonyhall/internal/modules/files/postgres"
	gitmem "colonyhall/internal/modules/git/memory"
	gitpg "colonyhall/internal/modules/git/postgres"
	hostmem "colonyhall/internal/modules/hosts/memory"
	hostpg "colonyhall/internal/modules/hosts/postgres"
	idmem "colonyhall/internal/modules/identity/memory"
	idpg "colonyhall/internal/modules/identity/postgres"
	qmem "colonyhall/internal/modules/question/memory"
	qpg "colonyhall/internal/modules/question/postgres"
	runmem "colonyhall/internal/modules/runs/memory"
	runpg "colonyhall/internal/modules/runs/postgres"
	schedmem "colonyhall/internal/modules/schedule/memory"
	schedpg "colonyhall/internal/modules/schedule/postgres"
	"colonyhall/internal/platform/db"
	"colonyhall/migrations"
)

func TestMemory(t *testing.T) {
	Run(t, func(t *testing.T) Backend {
		return Backend{Tx: &db.Memory{}, Events: evmem.New(), Identity: idmem.New(),
			Conversation: convmem.New(), Delivery: delmem.New(), Audit: audmem.New(), Hosts: hostmem.New(),
			Runs: runmem.New(), Schedule: schedmem.New(), Approval: appmem.New(), Wiki: wikimem.New(),
			Question: qmem.New(), Files: filesmem.NewStore(), Git: gitmem.New(), Preview: previewmem.New(),
			AgentRules: rulesmem.New(), Notes: notesmem.New(), Decisions: decisionsmem.New(), Leases: leasesmem.New()}
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
		_, err := pg.Pool.Exec(ctx, `TRUNCATE lease, decision, agent_note, agent_rules, preview, git_webhook_delivery, git_repo_channel, git_link, question, message_file, file, approval_request, agent_run, wiki_revision, wiki_page, audit_log, schedule, run_error, folder_grant, agent_host, host_pairing_code, host, reaction, delivery, consumer_offset, event, message, channel_member,
			channel, session, credential, api_token, actor, workspace RESTART IDENTITY CASCADE`)
		if err != nil {
			t.Fatal(err)
		}
		return Backend{Tx: pg, Events: &evpg.Store{DB: pg}, Identity: &idpg.Store{DB: pg},
			Conversation: &convpg.Store{DB: pg}, Delivery: &delpg.Store{DB: pg}, Audit: &audpg.Store{DB: pg},
			Hosts: &hostpg.Store{DB: pg}, Runs: &runpg.Store{DB: pg}, Schedule: &schedpg.Store{DB: pg},
			Approval: &apppg.Store{DB: pg}, Wiki: &wikipg.Store{DB: pg}, Question: &qpg.Store{DB: pg}, Files: &filespg.Store{DB: pg}, Git: &gitpg.Store{DB: pg}, Preview: &previewpg.Store{DB: pg},
			AgentRules: &rulespg.Store{DB: pg}, Notes: &notespg.Store{DB: pg}, Decisions: &decisionspg.Store{DB: pg}, Leases: &leasespg.Store{DB: pg}}
	})
}
