// Package server is the composition root: it picks the store backend,
// wires the modules together and builds the HTTP handler. Nothing else in
// the code base knows which concrete adapters are in use.
package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/eventlog"
	evmem "colonyhall/internal/eventlog/memory"
	evpg "colonyhall/internal/eventlog/postgres"
	"colonyhall/internal/gateway"
	"colonyhall/internal/httpserver"
	"colonyhall/internal/httpx"
	"colonyhall/internal/mcpserver"
	"colonyhall/internal/modules/agentrules"
	ruleshttp "colonyhall/internal/modules/agentrules/httpapi"
	rulesmem "colonyhall/internal/modules/agentrules/memory"
	rulespg "colonyhall/internal/modules/agentrules/postgres"
	"colonyhall/internal/modules/agents"
	agentsmem "colonyhall/internal/modules/agents/memory"
	agentspg "colonyhall/internal/modules/agents/postgres"
	"colonyhall/internal/modules/approval"
	apphttp "colonyhall/internal/modules/approval/httpapi"
	appmcp "colonyhall/internal/modules/approval/mcptools"
	appmem "colonyhall/internal/modules/approval/memory"
	apppg "colonyhall/internal/modules/approval/postgres"
	"colonyhall/internal/modules/audit"
	audhttp "colonyhall/internal/modules/audit/httpapi"
	audmem "colonyhall/internal/modules/audit/memory"
	audpg "colonyhall/internal/modules/audit/postgres"
	"colonyhall/internal/modules/compaction"
	compactionhttp "colonyhall/internal/modules/compaction/httpapi"
	compmem "colonyhall/internal/modules/compaction/memory"
	comppg "colonyhall/internal/modules/compaction/postgres"
	"colonyhall/internal/modules/conversation"
	convhttp "colonyhall/internal/modules/conversation/httpapi"
	convmcp "colonyhall/internal/modules/conversation/mcptools"
	convmem "colonyhall/internal/modules/conversation/memory"
	convpg "colonyhall/internal/modules/conversation/postgres"
	"colonyhall/internal/modules/decision"
	dechttp "colonyhall/internal/modules/decision/httpapi"
	decmcp "colonyhall/internal/modules/decision/mcptools"
	decmem "colonyhall/internal/modules/decision/memory"
	decpg "colonyhall/internal/modules/decision/postgres"
	"colonyhall/internal/modules/delegation"
	delegationhttp "colonyhall/internal/modules/delegation/httpapi"
	delegationmem "colonyhall/internal/modules/delegation/memory"
	delegationpg "colonyhall/internal/modules/delegation/postgres"
	"colonyhall/internal/modules/delivery"
	delhttp "colonyhall/internal/modules/delivery/httpapi"
	delmcp "colonyhall/internal/modules/delivery/mcptools"
	delmem "colonyhall/internal/modules/delivery/memory"
	delpg "colonyhall/internal/modules/delivery/postgres"
	"colonyhall/internal/modules/files"
	fileshttp "colonyhall/internal/modules/files/httpapi"
	filesmcp "colonyhall/internal/modules/files/mcptools"
	filesmem "colonyhall/internal/modules/files/memory"
	filespg "colonyhall/internal/modules/files/postgres"
	filesstorage "colonyhall/internal/modules/files/storage"
	"colonyhall/internal/modules/git"
	githttp "colonyhall/internal/modules/git/httpapi"
	gitmcp "colonyhall/internal/modules/git/mcptools"
	gitmem "colonyhall/internal/modules/git/memory"
	gitpg "colonyhall/internal/modules/git/postgres"
	"colonyhall/internal/modules/git/provider"
	"colonyhall/internal/modules/hosts"
	hosthttp "colonyhall/internal/modules/hosts/httpapi"
	hostmem "colonyhall/internal/modules/hosts/memory"
	hostpg "colonyhall/internal/modules/hosts/postgres"
	"colonyhall/internal/modules/identity"
	idhttp "colonyhall/internal/modules/identity/httpapi"
	idmem "colonyhall/internal/modules/identity/memory"
	idpg "colonyhall/internal/modules/identity/postgres"
	"colonyhall/internal/modules/lease"
	leaseshttp "colonyhall/internal/modules/lease/httpapi"
	leasesmcp "colonyhall/internal/modules/lease/mcptools"
	leasesmem "colonyhall/internal/modules/lease/memory"
	leasespg "colonyhall/internal/modules/lease/postgres"
	"colonyhall/internal/modules/notes"
	noteshttp "colonyhall/internal/modules/notes/httpapi"
	notesmcp "colonyhall/internal/modules/notes/mcptools"
	notesmem "colonyhall/internal/modules/notes/memory"
	notespg "colonyhall/internal/modules/notes/postgres"
	"colonyhall/internal/modules/planner"
	plannerfetch "colonyhall/internal/modules/planner/fetch"
	plannerhttp "colonyhall/internal/modules/planner/httpapi"
	plannermcp "colonyhall/internal/modules/planner/mcptools"
	plannermem "colonyhall/internal/modules/planner/memory"
	plannerpg "colonyhall/internal/modules/planner/postgres"
	"colonyhall/internal/modules/preview"
	previewhttp "colonyhall/internal/modules/preview/httpapi"
	previewmcp "colonyhall/internal/modules/preview/mcptools"
	previewmem "colonyhall/internal/modules/preview/memory"
	previewpg "colonyhall/internal/modules/preview/postgres"
	"colonyhall/internal/modules/question"
	qhttp "colonyhall/internal/modules/question/httpapi"
	qmcp "colonyhall/internal/modules/question/mcptools"
	qmem "colonyhall/internal/modules/question/memory"
	qpg "colonyhall/internal/modules/question/postgres"
	"colonyhall/internal/modules/runs"
	runhttp "colonyhall/internal/modules/runs/httpapi"
	runmem "colonyhall/internal/modules/runs/memory"
	runpg "colonyhall/internal/modules/runs/postgres"
	"colonyhall/internal/modules/schedule"
	schedhttp "colonyhall/internal/modules/schedule/httpapi"
	schedmem "colonyhall/internal/modules/schedule/memory"
	schedpg "colonyhall/internal/modules/schedule/postgres"
	"colonyhall/internal/modules/search"
	"colonyhall/internal/modules/search/embedhttp"
	searchhttp "colonyhall/internal/modules/search/httpapi"
	searchmcp "colonyhall/internal/modules/search/mcptools"
	searchmem "colonyhall/internal/modules/search/memory"
	searchpg "colonyhall/internal/modules/search/postgres"
	"colonyhall/internal/modules/sidebar"
	sidebarhttp "colonyhall/internal/modules/sidebar/httpapi"
	sidebarmem "colonyhall/internal/modules/sidebar/memory"
	sidebarpg "colonyhall/internal/modules/sidebar/postgres"
	"colonyhall/internal/modules/wiki"
	wikihttp "colonyhall/internal/modules/wiki/httpapi"
	wikimcp "colonyhall/internal/modules/wiki/mcptools"
	wikimem "colonyhall/internal/modules/wiki/memory"
	wikipg "colonyhall/internal/modules/wiki/postgres"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/migrations"
)

// maxSummaryBytes bounds a compaction summary read for the search index.
const maxSummaryBytes = 1 << 20

// Version is set at build time with -ldflags "-X colonyhall/internal/server.Version=…".
var Version = "dev"

// Options configures New.
type Options struct {
	// HTTPTimeouts bounds ordinary requests (not WebSockets, not /mcp).
	HTTPTimeouts httpserver.Timeouts

	Store       string // "postgres" or "memory"
	DatabaseURL string
	Clock       clock.Clock   // default: system clock
	IDs         ids.Generator // default: UUIDv7
	Logger      *slog.Logger
	// OriginPatterns allows cross-origin WebSocket clients.
	OriginPatterns []string
	// SecureCookies marks the session cookie HTTPS-only.
	SecureCookies bool
	// FilesDir is the root directory for uploaded files (default: "./data/files").
	FilesDir string
	// MaxUploadMB is the maximum upload size in megabytes (default: 25).
	MaxUploadMB int
	// StorageBackend overrides the default file storage adapter.
	StorageBackend files.StorageBackend
	// FilesStore overrides the default files metadata store.
	FilesStore files.Store

	// Gitea integration options (optional: if omitted, git module is inactive)
	GiteaURL           string
	GiteaToken         string
	GiteaPublicURL     string
	GiteaWebhookSecret string
	GitProvider        provider.GitProvider

	// CalendarRefresh is how often subscribed calendars are fetched
	// (default 15 minutes); CalendarFetcher replaces the safe fetcher in tests.
	CalendarRefresh time.Duration
	CalendarFetcher planner.Fetcher

	// Search (round 36a): the embedding server (the Raspberry Pi) and the
	// reranker (the homelab); empty means keyword search only / no rerank.
	EmbedURL  string
	RerankURL string
	RerankTop int
	// Embedder and Reranker override the URLs (tests).
	Embedder search.Embedder
	Reranker search.Reranker
}

// App is a wired-up Colonyhall server.
type App struct {
	Handler      http.Handler
	Identity     *identity.Service
	Conversation *conversation.Service
	Delivery     *delivery.Service
	Audit        *audit.Service
	Hosts        *hosts.Service
	Schedule     *schedule.Service
	Approval     *approval.Service
	Wiki         *wiki.Service
	Question     *question.Service
	Files        *files.Service
	Git          *git.Service
	Preview      *preview.Service
	AgentRules   *agentrules.Service
	Notes        *notes.Service
	Planner      *planner.Service
	Decisions    *decision.Service
	Leases       *lease.Service
	Sidebar      *sidebar.Service
	Compaction   *compaction.Service
	Delegation   *delegation.Service
	Runs         *runs.Service
	Search       *search.Service
	Indexer      *search.Indexer
	Gateway      *gateway.Gateway
	HostGateway  *gateway.HostGateway
	HostFs       *hosts.FsRelay
	HostLogin    *hosts.LoginRelay
	logger       *slog.Logger
	calRefresh   time.Duration
	Consumers    []*eventlog.Consumer
	Events       eventlog.Store
	Bus          bus.Bus
	close        func()
}

// New builds the application. For Store=postgres it also applies migrations.
func New(ctx context.Context, opt Options) (*App, error) {
	if opt.Clock == nil {
		opt.Clock = clock.System{}
	}
	if opt.IDs == nil {
		opt.IDs = ids.V7{}
	}
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	b := bus.NewInProc()

	var (
		tx             db.TxRunner
		events         eventlog.Store
		idStore        identity.Store
		cvStore        conversation.Store
		dlStore        delivery.Store
		auStore        audit.Store
		hoStore        hosts.Store
		ruStore        runs.Store
		scStore        schedule.Store
		apStore        approval.Store
		wiStore        wiki.Store
		fiStore        files.Store
		qStore         question.Store
		storageBackend files.StorageBackend
		ready          = func(context.Context) error { return nil }
		closeAll       = func() {}
		giStore        git.Store
		prStore        preview.Store
		rlStore        agentrules.Store
		ntStore        notes.Store
		agStore        agents.Store
		plStore        planner.Store
		dcStore        decision.Store
		lsStore        lease.Store
		sbStore        sidebar.Store
		cmStore        compaction.Store
		dgStore        delegation.Store
		seStore        search.Store
	)
	switch opt.Store {
	case "memory":
		tx = &db.Memory{}
		events, idStore, cvStore, dlStore, auStore = evmem.New(), idmem.New(), convmem.New(), delmem.New(), audmem.New()
		hoStore = hostmem.New()
		ruStore = runmem.New()
		scStore = schedmem.New()
		apStore = appmem.New()
		wiStore = wikimem.New()
		qStore = qmem.New()
		fiStore = filesmem.NewStore()
		storageBackend = filesmem.NewStorage()
		giStore = gitmem.New()
		prStore = previewmem.New()
		rlStore = rulesmem.New()
		ntStore = notesmem.New()
		agStore = agentsmem.New()
		plStore = plannermem.New()
		dcStore = decmem.New()
		lsStore = leasesmem.New()
		sbStore = sidebarmem.New()
		cmStore = compmem.NewStore()
		dgStore = delegationmem.New()
		seStore = searchmem.New()
	case "postgres":
		pg, err := db.Open(ctx, opt.DatabaseURL)
		if err != nil {
			return nil, err
		}
		if err := migrations.Up(ctx, pg.Pool); err != nil {
			pg.Pool.Close()
			return nil, err
		}
		tx = pg
		events, idStore, cvStore, dlStore = &evpg.Store{DB: pg}, &idpg.Store{DB: pg}, &convpg.Store{DB: pg}, &delpg.Store{DB: pg}
		auStore = &audpg.Store{DB: pg}
		hoStore = &hostpg.Store{DB: pg}
		ruStore = &runpg.Store{DB: pg}
		scStore = &schedpg.Store{DB: pg}
		apStore = &apppg.Store{DB: pg}
		wiStore = &wikipg.Store{DB: pg}
		qStore = &qpg.Store{DB: pg}
		fiStore = &filespg.Store{DB: pg}
		filesDir := opt.FilesDir
		if filesDir == "" {
			filesDir = "./data/files"
		}
		storageBackend, err = filesstorage.NewFS(filesDir)
		if err != nil {
			pg.Pool.Close()
			return nil, err
		}
		giStore = &gitpg.Store{DB: pg}
		prStore = &previewpg.Store{DB: pg}
		rlStore = &rulespg.Store{DB: pg}
		ntStore = &notespg.Store{DB: pg}
		agStore = &agentspg.Store{DB: pg}
		plStore = &plannerpg.Store{DB: pg}
		dcStore = &decpg.Store{DB: pg}
		lsStore = &leasespg.Store{DB: pg}
		sbStore = &sidebarpg.Store{DB: pg}
		cmStore = &comppg.Store{DB: pg}
		dgStore = &delegationpg.Store{DB: pg}
		seStore = &searchpg.Store{DB: pg}
		ready = func(ctx context.Context) error { return pg.Pool.Ping(ctx) }
		closeAll = pg.Pool.Close
	default:
		return nil, fmt.Errorf("server: unknown store %q", opt.Store)
	}

	if opt.StorageBackend != nil {
		storageBackend = opt.StorageBackend
	}
	if opt.FilesStore != nil {
		fiStore = opt.FilesStore
	}

	idSvc := &identity.Service{Store: idStore, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	cvSvc := &conversation.Service{Store: cvStore, Directory: idSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	dlSvc := &delivery.Service{Store: dlStore, Conversation: cvSvc, Directory: idSvc, IDs: opt.IDs, Clock: opt.Clock}
	auSvc := &audit.Service{Store: auStore, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	hoSvc := &hosts.Service{Store: hoStore, Directory: idSvc, Tokens: idSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	// An agent without its own colour shows its machine's (round 33).
	idSvc.HostColors = hoSvc.AgentHostColors
	scSvc := &schedule.Service{Store: scStore, Conversation: cvSvc, Actors: idSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	apSvc := &approval.Service{Store: apStore, Conversation: cvSvc, Agents: idSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	wiSvc := &wiki.Service{Store: wiStore, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	qSvc := &question.Service{Store: qStore, Conversation: cvSvc, Directory: idSvc, Hosts: hoSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	fiSvc := &files.Service{
		Store:        fiStore,
		Storage:      storageBackend,
		Conversation: cvSvc,
		Actors:       idSvc,
		Hosts:        hoSvc,
		Events:       events,
		Bus:          b,
		Clock:        opt.Clock,
		IDs:          opt.IDs,
		Tx:           tx,
		MaxUploadMB:  opt.MaxUploadMB,
		DetectMime:   http.DetectContentType,
	}
	cvSvc.Attachments = fiSvc

	gitProv := opt.GitProvider
	if gitProv == nil && opt.GiteaURL != "" {
		p, err := provider.NewGitea(opt.GiteaURL, opt.GiteaToken, opt.GiteaPublicURL)
		if err != nil {
			return nil, fmt.Errorf("server: gitea config: %w", err)
		}
		gitProv = p
	}
	gitSvc := &git.Service{
		Store:         giStore,
		Provider:      gitProv,
		Conversation:  cvSvc,
		Identity:      idSvc,
		Clock:         opt.Clock,
		WebhookSecret: opt.GiteaWebhookSecret,
	}
	cvSvc.LinkCards = gitSvc

	prSvc := &preview.Service{
		Store:        prStore,
		Hosts:        hoSvc,
		Conversation: cvSvc,
		Directory:    idSvc,
		Events:       events,
		Bus:          b,
		Tx:           tx,
		IDs:          opt.IDs,
		Clock:        opt.Clock,
	}
	hoSvc.Previews = prSvc
	rlSvc := &agentrules.Service{
		Store:     rlStore,
		Directory: idSvc,
		Events:    events,
		Bus:       b,
		Tx:        tx,
		IDs:       opt.IDs,
		Clock:     opt.Clock,
	}
	ntSvc := notes.NewService(ntStore, cvSvc, idSvc, opt.Clock, opt.IDs)
	dcSvc := decision.NewService(dcStore, wiSvc, cvSvc, tx, opt.Clock, opt.IDs)
	dcSvc.Events, dcSvc.Bus = events, b
	lsSvc := lease.NewService(lsStore, idSvc, events, b, tx, opt.Clock, opt.IDs)
	sbSvc := sidebar.NewService(sbStore, cvSvc, opt.Clock, opt.IDs)
	cmSvc := &compaction.Service{
		Store:        cmStore,
		Conversation: cvSvc,
		Directory:    idSvc,
		Files:        fiSvc,
		Events:       events,
		Bus:          b,
		Clock:        opt.Clock,
		IDs:          opt.IDs,
		Tx:           tx,
	}
	cvSvc.Compactions = cmStore

	dgSvc := &delegation.Service{
		Store:        dgStore,
		Conversation: cvSvc,
		Directory:    idSvc,
		Hosts:        hoSvc,
		Question:     qSvc,
		Delivery:     dlStore,
		Events:       events,
		Bus:          b,
		Tx:           tx,
		IDs:          opt.IDs,
		Clock:        opt.Clock,
	}
	dlSvc.Approval = dgSvc
	dlSvc.FullAccess = hoSvc
	qSvc.Delegation = dgSvc
	// Agent lifecycle (round 33): one configuration at creation, deleting
	// archives. The delegation and inbox clean-ups run in its transaction.
	agSvc := &agents.Service{
		Store: agStore, Identity: idSvc, Hosts: hoSvc, Channels: cvSvc, Rules: rlSvc,
		RevokeDelegations: func(ctx context.Context, by identity.Actor, agentID ids.ID) error {
			grants, err := dgSvc.ListGrants(ctx, by, true)
			if err != nil {
				return err
			}
			for _, g := range grants {
				if g.AgentAID == agentID || g.AgentBID == agentID {
					if _, err := dgSvc.RevokeGrant(ctx, by, g.ID); err != nil {
						return err
					}
				}
			}
			return nil
		},
		AckInbox: func(ctx context.Context, agentID ids.ID) error {
			_, err := dlSvc.AckAllFor(ctx, agentID)
			return err
		},
		Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock,
	}
	rlSvc.OwnInstructions = agSvc.OwnInstructions

	embedder, reranker := opt.Embedder, opt.Reranker
	if embedder == nil && opt.EmbedURL != "" {
		c, err := embedhttp.New(opt.EmbedURL)
		if err != nil {
			return nil, fmt.Errorf("server: COLONY_EMBED_URL: %w", err)
		}
		embedder = c
	}
	if reranker == nil && opt.RerankURL != "" {
		c, err := embedhttp.New(opt.RerankURL)
		if err != nil {
			return nil, fmt.Errorf("server: COLONY_RERANK_URL: %w", err)
		}
		reranker = c
	}
	seSvc := &search.Service{Store: seStore, Conv: cvSvc, Directory: idSvc, Embedder: embedder, Reranker: reranker,
		RerankTop: opt.RerankTop, IDs: opt.IDs, Clock: opt.Clock, Logger: opt.Logger}
	seIdx := &search.Indexer{Store: seStore, Conv: cvSvc, Directory: idSvc, Wiki: wiSvc, Decisions: dcSvc,
		Summaries: func(ctx context.Context, fileID ids.ID) (string, error) {
			_, rc, err := fiSvc.OpenContentAuthorized(ctx, fileID)
			if err != nil {
				return "", err
			}
			defer func() { _ = rc.Close() }()
			raw, err := io.ReadAll(io.LimitReader(rc, maxSummaryBytes))
			return string(raw), err
		},
		IDs: opt.IDs, Clock: opt.Clock, Logger: opt.Logger, Indexed: seSvc.Wake}

	consumers := []*eventlog.Consumer{
		{Name: delivery.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: dlSvc.HandleEvent, Logger: opt.Logger},
		{Name: audit.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: auSvc.HandleEvent, Logger: opt.Logger},
		{Name: approval.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: apSvc.HandleEvent, Logger: opt.Logger},
		{Name: git.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: gitSvc.HandleEvent, Logger: opt.Logger},
		{Name: preview.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: prSvc.HandleEvent, Logger: opt.Logger},
		{Name: lease.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: lsSvc.HandleEvent, Logger: opt.Logger},
		{Name: compaction.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: cmSvc.HandleEvent, Logger: opt.Logger},
		{Name: delegation.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: dgSvc.HandleEvent, Logger: opt.Logger},
		{Name: search.ConsumerName, Store: events, Tx: tx, Bus: b, Handle: seIdx.HandleEvent, Logger: opt.Logger},
	}

	mux := http.NewServeMux()
	idhttp.Register(mux, idSvc, idhttp.Options{SecureCookies: opt.SecureCookies})
	convhttp.Register(mux, cvSvc, idSvc)
	delhttp.Register(mux, dlSvc, idSvc)
	audhttp.Register(mux, auSvc, idSvc, idSvc)
	hosthttp.Register(mux, hoSvc, idSvc)
	// Host-level folder listing: the guard's own WebSocket (host token)
	// carries host.fs.request frames; see docs/handoff/33-host-fs-relay-notes.md.
	hostGw := &gateway.HostGateway{Auth: hoSvc, Logger: opt.Logger}
	hostFs := &hosts.FsRelay{Hosts: hoSvc, Pusher: hostGw}
	hosthttp.RegisterFs(mux, hoSvc, hostFs, idSvc)
	hostLogin := &hosts.LoginRelay{Hosts: hoSvc, Pusher: hostGw}
	hosthttp.RegisterLogin(mux, hoSvc, hostLogin, idSvc)
	mux.Handle("GET /ws/host", hostGw)
	wikihttp.Register(mux, wiSvc, idSvc)
	previewhttp.Register(mux, prSvc, idSvc, hoSvc)
	schedhttp.Register(mux, scSvc, idSvc)
	ruSvc := &runs.Service{Store: ruStore, Agents: idSvc, Conversation: cvSvc, Hosts: hoSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	runhttp.Register(mux, ruSvc, idSvc)
	apphttp.Register(mux, apSvc, idSvc)
	qhttp.Register(mux, qSvc, idSvc, cvSvc, idSvc)
	fileshttp.Register(mux, fiSvc, idSvc)
	githttp.Register(mux, gitSvc, idSvc)
	ruleshttp.Register(mux, rlSvc, idSvc)
	noteshttp.Register(mux, ntSvc, idSvc)
	calFetcher := opt.CalendarFetcher
	if calFetcher == nil {
		calFetcher = plannerfetch.Safe{}
	}
	plSvc := &planner.Service{Store: plStore, Directory: idSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock,
		Fetcher: calFetcher}
	plannerhttp.Register(mux, plSvc, idSvc)
	dechttp.Register(mux, dcSvc, idSvc)
	leaseshttp.Register(mux, lsSvc, idSvc)
	sidebarhttp.Register(mux, sbSvc, idSvc)
	compactionhttp.Register(mux, cmSvc, idSvc)
	delegationhttp.Register(mux, dgSvc, idSvc)
	searchhttp.Register(mux, seSvc, idSvc)
	mcpHandler := mcpserver.Handler(idSvc, Version, func(s *mcp.Server, actor identity.Actor, r *http.Request) {
		current := func(ctx context.Context) (identity.Actor, error) { return idSvc.Current(ctx, actor.ID) }
		delmcp.Register(s, dlSvc, current)
		convmcp.Register(s, cvSvc, idSvc, current, r)
		appmcp.Register(s, apSvc, current, r.Header)
		wikimcp.Register(s, wiSvc, current)
		qmcp.Register(s, qSvc, cvSvc, current)
		filesmcp.Register(s, fiSvc, cvSvc, current)
		gitmcp.Register(s, gitSvc, current)
		previewmcp.Register(s, prSvc, current)
		notesmcp.Register(s, ntSvc, current, r)
		plannermcp.Register(s, plSvc, current)
		decmcp.Register(s, dcSvc, current, r)
		leasesmcp.Register(s, lsSvc, current)
		searchmcp.Register(s, seSvc, current)
	}, func(ctx context.Context, actor identity.Actor) (string, error) {
		text, err := rlSvc.InstructionsForActor(ctx, actor)
		if err != nil || actor.Kind != identity.KindAgent {
			return text, err
		}
		return text + searchmcp.Hint, nil
	}, opt.Logger)
	for _, method := range []string{"GET", "POST", "DELETE"} {
		mux.Handle(method+" /mcp", mcpHandler)
	}
	gw := &gateway.Gateway{
		Events:         events,
		Bus:            b,
		Auth:           idSvc,
		Conversation:   cvSvc,
		Clock:          opt.Clock,
		Logger:         opt.Logger,
		OriginPatterns: opt.OriginPatterns,
	}
	fiSvc.FsPusher = gw
	fiSvc.Logger = opt.Logger
	mux.Handle("GET /ws/ui", gw)
	mux.Handle("POST /api/v1/agent/activity", httpx.RequireAuth(idSvc, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.AgentActivityRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var threadRootID ids.ID
		if in.ThreadRootId != nil {
			threadRootID = ids.ID(*in.ThreadRootId)
		}
		err := gw.ReportActivity(r.Context(), httpx.Actor(r), ids.ID(in.ChannelId), threadRootID, string(in.State))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.Handle("POST /api/v1/agent/progress", httpx.RequireAuth(idSvc, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.AgentProgressRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := gw.ReportProgress(r.Context(), httpx.Actor(r), in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.Handle("GET /api/v1/channels/{channelId}/progress", httpx.RequireAuth(idSvc, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chID, err := ids.Parse(r.PathValue("channelId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid channel id"})
			return
		}
		runs, err := gw.ChannelProgress(r.Context(), httpx.Actor(r), chID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.ChannelProgressResponse{Runs: runs})
	})))
	registerOverview(mux, idSvc, cvSvc, hoSvc, ruSvc, ntSvc, lsSvc, qSvc, dlSvc, gw, opt.Clock)
	registerAgents(mux, idSvc, agSvc)
	registerStatus(mux, statusDeps{Identity: idSvc, Hosts: hoSvc, Schedules: scStore, Compactions: cmStore, Events: events,
		Consumers: consumers, Gateway: gw, HostGateway: hostGw, Ready: ready, Clock: opt.Clock, StartedAt: opt.Clock.Now(), GiteaOn: gitProv != nil,
		WebhookOn: opt.GiteaWebhookSecret != "", MaxUploadMB: cmp.Or(opt.MaxUploadMB, files.DefaultMaxUploadMB), Search: seSvc})

	return &App{
		Handler:      httpserver.NewWithTimeouts(mux, ready, opt.Logger, opt.HTTPTimeouts),
		Identity:     idSvc,
		Conversation: cvSvc,
		Delivery:     dlSvc,
		Audit:        auSvc,
		Hosts:        hoSvc,
		Schedule:     scSvc,
		Approval:     apSvc,
		Wiki:         wiSvc,
		Question:     qSvc,
		Files:        fiSvc,
		Git:          gitSvc,
		Preview:      prSvc,
		AgentRules:   rlSvc,
		Notes:        ntSvc,
		Planner:      plSvc,
		calRefresh:   opt.CalendarRefresh,
		Decisions:    dcSvc,
		Leases:       lsSvc,
		Sidebar:      sbSvc,
		Compaction:   cmSvc,
		Delegation:   dgSvc,
		Runs:         ruSvc,
		Search:       seSvc,
		Indexer:      seIdx,
		Gateway:      gw,
		HostGateway:  hostGw,
		HostFs:       hostFs,
		HostLogin:    hostLogin,
		logger:       opt.Logger,
		Consumers:    consumers,
		Events:       events,
		Bus:          b,
		close:        closeAll,
	}, nil
}

// Run starts the background consumers and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Schedule.Loop(ctx, 15*time.Second, a.logger.Info)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Question.Loop(ctx, 15*time.Second, a.logger.Info)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Planner.Loop(ctx, a.calRefresh)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if a.Files != nil {
					_, _ = a.Files.SweepPending(ctx)
				}
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Search.RunEmbedder(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		// The search log is kept 30 days.
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			if n, err := a.Search.PurgeLogs(ctx); err != nil && ctx.Err() == nil {
				a.logger.Warn("search: log purge failed", "err", err)
			} else if n > 0 {
				a.logger.Info("search: old log rows removed", "rows", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	for _, c := range a.Consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Run(ctx)
		}()
	}
	wg.Wait()
	return nil
}

// Drain runs every consumer until it has processed all current events.
// Tests use it to make the asynchronous parts deterministic.
func (a *App) Drain(ctx context.Context) error {
	for pass := 0; pass < 10; pass++ {
		var anyProcessed bool
		for _, c := range a.Consumers {
			offsetBefore, _ := c.Store.Offset(ctx, c.Name)
			if err := c.Drain(ctx); err != nil {
				return err
			}
			offsetAfter, _ := c.Store.Offset(ctx, c.Name)
			if offsetAfter > offsetBefore {
				anyProcessed = true
			}
		}
		if !anyProcessed {
			break
		}
	}
	return nil
}

// Close releases resources.
func (a *App) Close() { a.close() }

// Development seed: fixed credentials so a local stack is usable immediately.
// Never enable on a reachable server.
const (
	DevAdminToken    = "col_dev_admin_token_do_not_use_in_production"
	DevAgentToken    = "col_dev_agent_token_do_not_use_in_production"
	DevAdminPassword = "colonyhall-dev-password"
)

// SeedDev creates a workspace with @norbi (owner), @fake-agent and #general,
// unless the admin token already works.
func (a *App) SeedDev(ctx context.Context) error {
	if owner, err := a.Identity.Authenticate(ctx, DevAdminToken); err == nil {
		// Already seeded; databases seeded before passwords existed get one now.
		if _, err := a.Identity.Store.Credential(ctx, owner.ID); errors.Is(err, db.ErrNotFound) {
			return a.Identity.ResetPassword(ctx, owner.ID, DevAdminPassword)
		}
		return nil
	} else if !errors.Is(err, identity.ErrUnauthorized) {
		return err
	}
	_, owner, _, err := a.Identity.Bootstrap(ctx, "colonyhall", "norbi", "Norbi", DevAdminToken)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := a.Identity.ResetPassword(ctx, owner.ID, DevAdminPassword); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	agent, _, err := a.Identity.CreateAgentActor(ctx, owner, identity.AgentActorInput{
		Handle: "fake-agent", DisplayName: "Fake agent", WithToken: true, Secret: DevAgentToken,
	})
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	ch, err := a.Conversation.CreateChannel(ctx, owner, "general", "", conversation.KindPublic)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return a.Conversation.AddMember(ctx, owner, ch.ID, agent.ID, true)
}
