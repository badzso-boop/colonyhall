package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"colonyhall/internal/modules/search"
	"colonyhall/internal/modules/search/fakeembed"
	"colonyhall/internal/platform/config"
	"colonyhall/internal/server"
)

// openApp builds the server from the environment, as `colonyhall` would,
// without serving HTTP. embedURL overrides COLONY_EMBED_URL when set.
func openApp(ctx context.Context, embedURL string) (*server.App, error) {
	cfg, err := config.FromEnv()
	if err != nil {
		return nil, err
	}
	if embedURL != "" {
		cfg.EmbedURL = embedURL
	}
	return server.New(ctx, server.Options{Store: cfg.Store, DatabaseURL: cfg.DatabaseURL, FilesDir: cfg.FilesDir,
		MaxUploadMB: cfg.MaxUploadMB, EmbedURL: cfg.EmbedURL, RerankURL: cfg.RerankURL, RerankTop: cfg.RerankTop})
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// ragBackfill (re)builds the search index from the event log:
//
//	colonyhall rag-backfill [-dry-run] [-restart] [-embed] [-embed-url URL]
func ragBackfill(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("rag-backfill", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "only count what would be indexed")
	restart := fs.Bool("restart", false, "read the event log from the start (unchanged chunks keep their embeddings)")
	embed := fs.Bool("embed", false, "also embed the chunks here instead of leaving them to the running server")
	embedURL := fs.String("embed-url", "", "embedding server for -embed (default COLONY_EMBED_URL)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	app, err := openApp(ctx, *embedURL)
	if err != nil {
		return err
	}
	defer app.Close()
	rep, err := app.SearchBackfill(ctx, *restart, *embed, *dry, func(line string) { _, _ = fmt.Fprintln(out, line) })
	if err != nil {
		return err
	}
	if *dry {
		_, _ = fmt.Fprintf(out, "dry run: %d events, %d of an indexed kind, %d workspaces, %d decisions\n",
			rep.Events, rep.Sources, rep.Workspaces, rep.Decisions)
		return nil
	}
	_, _ = fmt.Fprintf(out, "done: %d events read, %d decisions indexed, %d chunks embedded here, %d waiting for the embedder\n",
		rep.Events, rep.Decisions, rep.Embedded, rep.Pending)
	return nil
}

// ragReembed re-queues every vector not made by the embedding server's
// current model (after a model change):
//
//	colonyhall rag-reembed
func ragReembed(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("rag-reembed", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	app, err := openApp(ctx, "")
	if err != nil {
		return err
	}
	defer app.Close()
	n, model, err := app.Search.Reembed(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%d chunks queued for %s; the running server embeds them\n", n, model)
	return nil
}

// ragEval runs an evaluation set and prints the metrics side by side:
//
//	colonyhall rag-eval -set testdata/rag-eval/synthetic.json        (seeded, throwaway, in memory)
//	colonyhall rag-eval -set ~/eval/valodi.json -as norbi            (the configured database)
func ragEval(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("rag-eval", flag.ContinueOnError)
	setPath := fs.String("set", "", "the evaluation set (JSON, see docs/11-rag-terv.md)")
	as := fs.String("as", "", "search as this @handle (a real set; default for a seeded set: the dev agent)")
	embedURL := fs.String("embed-url", "", "a seeded set: the real embedding server instead of the fake")
	rerankURL := fs.String("rerank-url", "", "a seeded set: the real reranker instead of the fake")
	seedDB := fs.String("seed-db", "", "a seeded set: an empty, throwaway Postgres to seed instead of memory (it is filled)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *setPath == "" {
		return errors.New("rag-eval: -set is required")
	}
	f, err := os.Open(*setPath)
	if err != nil {
		return err
	}
	set, err := search.ReadEvalSet(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	metrics, err := EvalSet(ctx, set, *as, *embedURL, *rerankURL, *seedDB)
	if err != nil {
		return err
	}
	name := set.Name
	if name == "" {
		name = *setPath
	}
	_, _ = fmt.Fprint(out, search.FormatMetrics(name, metrics))
	return nil
}

// EvalSet runs a set: a seeded one on a throwaway in-memory server, a real
// one on the configured database.
func EvalSet(ctx context.Context, set search.EvalSet, as, embedURL, rerankURL, seedDB string) ([]search.EvalMetrics, error) {
	if set.Seed == nil {
		if as == "" {
			return nil, errors.New("rag-eval: a set without seed runs on the database and needs -as HANDLE")
		}
		app, err := openApp(ctx, embedURL)
		if err != nil {
			return nil, err
		}
		defer app.Close()
		actor, err := app.Identity.FindHuman(ctx, as, "")
		if err != nil {
			return nil, err
		}
		return app.RunEval(ctx, set, actor, nil)
	}
	opt := server.Options{Store: "memory", EmbedURL: embedURL, RerankURL: rerankURL}
	if seedDB != "" {
		opt.Store, opt.DatabaseURL = "postgres", seedDB
	}
	if embedURL == "" {
		opt.Embedder = &fakeembed.Fake{}
	}
	if rerankURL == "" {
		opt.Reranker = &fakeembed.Fake{}
	}
	app, err := server.New(ctx, opt)
	if err != nil {
		return nil, err
	}
	defer app.Close()
	if err := app.SeedDev(ctx); err != nil {
		return nil, err
	}
	refs, err := app.EvalSeed(ctx, set.Seed)
	if err != nil {
		return nil, err
	}
	token := server.DevAgentToken
	if as != "" && as != "fake-agent" {
		token = server.DevAdminToken
	}
	actor, err := app.Identity.Authenticate(ctx, token)
	if err != nil {
		return nil, err
	}
	return app.RunEval(ctx, set, actor, refs)
}
