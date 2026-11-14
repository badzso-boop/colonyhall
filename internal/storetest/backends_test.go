package storetest

import (
	activitymem "colonyhall/internal/modules/activity/memory"
	activitypg "colonyhall/internal/modules/activity/postgres"
	rulesmem "colonyhall/internal/modules/agentrules/memory"
	rulespg "colonyhall/internal/modules/agentrules/postgres"
	agentsmem "colonyhall/internal/modules/agents/memory"
	agentspg "colonyhall/internal/modules/agents/postgres"
	compmem "colonyhall/internal/modules/compaction/memory"
	comppg "colonyhall/internal/modules/compaction/postgres"
	dbstatsmem "colonyhall/internal/modules/dbstats/memory"
	dbstatspg "colonyhall/internal/modules/dbstats/postgres"
	decisionsmem "colonyhall/internal/modules/decision/memory"
	decisionspg "colonyhall/internal/modules/decision/postgres"
	delegationmem "colonyhall/internal/modules/delegation/memory"
	delegationpg "colonyhall/internal/modules/delegation/postgres"
	draftmem "colonyhall/internal/modules/draft/memory"
	draftpg "colonyhall/internal/modules/draft/postgres"
	leasesmem "colonyhall/internal/modules/lease/memory"
	leasespg "colonyhall/internal/modules/lease/postgres"
	notesmem "colonyhall/internal/modules/notes/memory"
	notespg "colonyhall/internal/modules/notes/postgres"
	plannermem "colonyhall/internal/modules/planner/memory"
	plannerpg "colonyhall/internal/modules/planner/postgres"
	previewmem "colonyhall/internal/modules/preview/memory"
	previewpg "colonyhall/internal/modules/preview/postgres"
	savedmem "colonyhall/internal/modules/saved/memory"
	savedpg "colonyhall/internal/modules/saved/postgres"
	sidebarmem "colonyhall/internal/modules/sidebar/memory"
	sidebarpg "colonyhall/internal/modules/sidebar/postgres"
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
	searchmem "colonyhall/internal/modules/search/memory"
	searchpg "colonyhall/internal/modules/search/postgres"
	"colonyhall/internal/platform/db"
	"colonyhall/migrations"
)

func TestMemory(t *testing.T) {
	Run(t, func(t *testing.T) Backend {
		return Backend{Tx: &db.Memory{}, Events: evmem.New(), Identity: idmem.New(),
			Conversation: convmem.New(), Delivery: delmem.New(), Audit: audmem.New(), Hosts: hostmem.New(),
			Runs: runmem.New(), Schedule: schedmem.New(), Approval: appmem.New(), Wiki: wikimem.New(),
			Question: qmem.New(), Files: filesmem.NewStore(), Git: gitmem.New(), Preview: previewmem.New(),
			AgentRules: rulesmem.New(), Notes: notesmem.New(), Agents: agentsmem.New(), Planner: plannermem.New(), Decisions: decisionsmem.New(), Leases: leasesmem.New(),
			Sidebar: sidebarmem.New(), Compaction: compmem.NewStore(), Delegation: delegationmem.New(), Search: searchmem.New(), DBStats: dbstatsmem.New(), Drafts: draftmem.New(), Saved: savedmem.New(), Activity: activitymem.New()}
	})
}

// TestPostgres runs the suite against a real database. It needs
// COLONY_TEST_DATABASE_URL (see `make test-pg`) and wipes that database.
// It connects as a role without BYPASSRLS that owns the tables, like the
// server, so every store call that forgets its workspace scope fails.
func TestPostgres(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	url = AppRoleDatabase(t, url)
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
		_, err := pg.Pool.Exec(ctx, `TRUNCATE emoji_use, activity_item, saved_item, message_pin, draft, auth_failure, search_chunk_text, search_log, db_size_sample, workspace_gitea, model_price, workspace_currency, agent_instructions, planner_calendar_event, planner_calendar, planner_access, planner_task, terminal_event, delegation, compaction_unit, compaction, workspace_compaction_settings, sidebar_item, sidebar_section, lease, decision, agent_note, agent_rules, preview, git_webhook_delivery, git_repo_channel, git_link, question, message_file, file, approval_request, agent_run, wiki_revision, wiki_page, audit_log, schedule, run_error, folder_grant, agent_host, host_pairing_code, host, reaction, delivery, consumer_offset, event, message, channel_member,
			channel, session, credential, api_token, actor, workspace RESTART IDENTITY CASCADE`)
		if err != nil {
			t.Fatal(err)
		}
		return Backend{Tx: pg, Events: &evpg.Store{DB: pg}, Identity: &idpg.Store{DB: pg},
			Conversation: &convpg.Store{DB: pg}, Delivery: &delpg.Store{DB: pg}, Audit: &audpg.Store{DB: pg},
			Hosts: &hostpg.Store{DB: pg}, Runs: &runpg.Store{DB: pg}, Schedule: &schedpg.Store{DB: pg},
			Approval: &apppg.Store{DB: pg}, Wiki: &wikipg.Store{DB: pg}, Question: &qpg.Store{DB: pg}, Files: &filespg.Store{DB: pg}, Git: &gitpg.Store{DB: pg}, Preview: &previewpg.Store{DB: pg},
			AgentRules: &rulespg.Store{DB: pg}, Notes: &notespg.Store{DB: pg}, Agents: &agentspg.Store{DB: pg}, Planner: &plannerpg.Store{DB: pg}, Decisions: &decisionspg.Store{DB: pg}, Leases: &leasespg.Store{DB: pg},
			Sidebar: &sidebarpg.Store{DB: pg}, Compaction: &comppg.Store{DB: pg}, Delegation: &delegationpg.Store{DB: pg}, Search: &searchpg.Store{DB: pg}, DBStats: &dbstatspg.Store{DB: pg}, Drafts: &draftpg.Store{DB: pg}, Saved: &savedpg.Store{DB: pg}, Activity: &activitypg.Store{DB: pg}}
	})
}
