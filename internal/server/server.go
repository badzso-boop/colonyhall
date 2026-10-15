// Package server is the composition root: it picks the store backend,
// wires the modules together and builds the HTTP handler. Nothing else in
// the code base knows which concrete adapters are in use.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/eventlog"
	evmem "colonyhall/internal/eventlog/memory"
	evpg "colonyhall/internal/eventlog/postgres"
	"colonyhall/internal/gateway"
	"colonyhall/internal/httpserver"
	"colonyhall/internal/mcpserver"
	"colonyhall/internal/modules/conversation"
	convhttp "colonyhall/internal/modules/conversation/httpapi"
	convmcp "colonyhall/internal/modules/conversation/mcptools"
	convmem "colonyhall/internal/modules/conversation/memory"
	convpg "colonyhall/internal/modules/conversation/postgres"
	"colonyhall/internal/modules/delivery"
	delhttp "colonyhall/internal/modules/delivery/httpapi"
	delmcp "colonyhall/internal/modules/delivery/mcptools"
	delmem "colonyhall/internal/modules/delivery/memory"
	delpg "colonyhall/internal/modules/delivery/postgres"
	"colonyhall/internal/modules/identity"
	idhttp "colonyhall/internal/modules/identity/httpapi"
	idmem "colonyhall/internal/modules/identity/memory"
	idpg "colonyhall/internal/modules/identity/postgres"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/migrations"
)

// Version is set at build time with -ldflags "-X colonyhall/internal/server.Version=…".
var Version = "dev"

// Options configures New.
type Options struct {
	Store       string // "postgres" or "memory"
	DatabaseURL string
	Clock       clock.Clock   // default: system clock
	IDs         ids.Generator // default: UUIDv7
	Logger      *slog.Logger
	// OriginPatterns allows cross-origin WebSocket clients.
	OriginPatterns []string
}

// App is a wired-up Colonyhall server.
type App struct {
	Handler          http.Handler
	Identity         *identity.Service
	Conversation     *conversation.Service
	Delivery         *delivery.Service
	DeliveryConsumer *eventlog.Consumer
	Events           eventlog.Store
	Bus              bus.Bus
	close            func()
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
		tx       db.TxRunner
		events   eventlog.Store
		idStore  identity.Store
		cvStore  conversation.Store
		dlStore  delivery.Store
		ready    = func(context.Context) error { return nil }
		closeAll = func() {}
	)
	switch opt.Store {
	case "memory":
		tx = &db.Memory{}
		events, idStore, cvStore, dlStore = evmem.New(), idmem.New(), convmem.New(), delmem.New()
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
		ready = func(ctx context.Context) error { return pg.Pool.Ping(ctx) }
		closeAll = pg.Pool.Close
	default:
		return nil, fmt.Errorf("server: unknown store %q", opt.Store)
	}

	idSvc := &identity.Service{Store: idStore, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	cvSvc := &conversation.Service{Store: cvStore, Directory: idSvc, Events: events, Bus: b, Tx: tx, IDs: opt.IDs, Clock: opt.Clock}
	dlSvc := &delivery.Service{Store: dlStore, Conversation: cvSvc, Directory: idSvc, IDs: opt.IDs, Clock: opt.Clock}
	consumer := &eventlog.Consumer{Name: delivery.ConsumerName, Store: events, Tx: tx, Bus: b,
		Handle: dlSvc.HandleEvent, Logger: opt.Logger}

	mux := http.NewServeMux()
	idhttp.Register(mux, idSvc)
	convhttp.Register(mux, cvSvc, idSvc)
	delhttp.Register(mux, dlSvc, idSvc)
	mcpHandler := mcpserver.Handler(idSvc, Version, func(s *mcp.Server, actor identity.Actor) {
		delmcp.Register(s, dlSvc, actor)
		convmcp.Register(s, cvSvc, idSvc, actor)
	}, opt.Logger)
	for _, method := range []string{"GET", "POST", "DELETE"} {
		mux.Handle(method+" /mcp", mcpHandler)
	}
	mux.Handle("GET /ws/ui", &gateway.Gateway{Events: events, Bus: b, Auth: idSvc, Conversation: cvSvc,
		Logger: opt.Logger, OriginPatterns: opt.OriginPatterns})

	return &App{
		Handler:          httpserver.New(mux, ready, opt.Logger),
		Identity:         idSvc,
		Conversation:     cvSvc,
		Delivery:         dlSvc,
		DeliveryConsumer: consumer,
		Events:           events,
		Bus:              b,
		close:            closeAll,
	}, nil
}

// Run starts the background consumers and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	return a.DeliveryConsumer.Run(ctx)
}

// Close releases resources.
func (a *App) Close() { a.close() }

// Development seed: fixed tokens so a local stack is usable immediately.
// Never enable on a reachable server.
const (
	DevAdminToken = "col_dev_admin_token_do_not_use_in_production"
	DevAgentToken = "col_dev_agent_token_do_not_use_in_production"
)

// SeedDev creates a workspace with @norbi (owner), @fake-agent and #general,
// unless the admin token already works.
func (a *App) SeedDev(ctx context.Context) error {
	if _, err := a.Identity.Authenticate(ctx, DevAdminToken); err == nil {
		return nil
	} else if !errors.Is(err, identity.ErrUnauthorized) {
		return err
	}
	_, owner, _, err := a.Identity.Bootstrap(ctx, "colonyhall", "norbi", "Norbi", DevAdminToken)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	agent, _, err := a.Identity.CreateActor(ctx, owner, identity.NewActorInput{
		Kind: identity.KindAgent, Handle: "fake-agent", DisplayName: "Fake agent", Role: identity.RoleMember, Secret: DevAgentToken,
	})
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	ch, err := a.Conversation.CreateChannel(ctx, owner, "general", conversation.KindPublic)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return a.Conversation.AddMember(ctx, owner, ch.ID, agent.ID, true)
}
