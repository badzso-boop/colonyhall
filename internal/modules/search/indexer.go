package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretscan"
)

// Indexer keeps the index current from the event log. Every handler reads
// the source's current state, so replaying the whole log (a backfill)
// converges to the same index; a missing source is skipped, never an
// error that would stall the consumer.
type Indexer struct {
	Store     Store
	Conv      Conversation
	Directory Directory
	Wiki      Wiki
	Decisions Decisions
	Summaries SummaryReader
	IDs       ids.Generator
	Clock     Clock
	Logger    *slog.Logger
	// Indexed is called after chunks were written (wakes the embedder).
	Indexed func()
}

func (x *Indexer) log() *slog.Logger {
	if x.Logger == nil {
		return slog.Default()
	}
	return x.Logger
}

// HandleEvent is the consumer handler.
func (x *Indexer) HandleEvent(ctx context.Context, e eventlog.Event) error {
	var err error
	switch e.Type {
	case conversation.EventMessageCreated:
		var p conversation.MessageCreated
		if json.Unmarshal(e.Payload, &p) == nil {
			err = x.indexMessage(ctx, parseID(p.MessageID))
		}
	case conversation.EventMessageRetracted:
		var p conversation.MessageRetracted
		if json.Unmarshal(e.Payload, &p) == nil {
			err = x.Store.DeleteSource(ctx, SourceMessage, parseID(p.MessageID))
		}
	case wiki.EventCreated, wiki.EventAppended, wiki.EventEdited, wiki.EventDeleted, wiki.EventRestored:
		var p wiki.Event
		if json.Unmarshal(e.Payload, &p) == nil {
			err = x.indexWiki(ctx, e.WorkspaceID, p.Slug)
		}
	case decision.EventRecorded:
		var p decision.Recorded
		if json.Unmarshal(e.Payload, &p) == nil {
			d := decision.Decision{ID: parseID(p.DecisionID), Number: p.Number, WorkspaceID: e.WorkspaceID,
				AuthorID: parseID(p.AuthorID), ChannelID: parseID(p.ChannelID), Text: p.Text, Rationale: p.Rationale,
				Supersedes: p.Supersedes, CreatedAt: e.At}
			err = x.IndexDecision(ctx, d)
		}
	case compaction.EventDone:
		var p compaction.CompactionDone
		if json.Unmarshal(e.Payload, &p) == nil {
			err = x.indexSummary(ctx, e.WorkspaceID, p, e.At)
		}
	}
	return err
}

func parseID(s string) ids.ID {
	id, err := ids.Parse(s)
	if err != nil {
		return ids.Nil
	}
	return id
}

// missing reports a source that is gone; the handler skips it.
func missing(err error) bool {
	return errors.Is(err, db.ErrNotFound) || errors.Is(err, wiki.ErrNotFound)
}

func (x *Indexer) handle(ctx context.Context, id ids.ID) string {
	if id == ids.Nil {
		return "?"
	}
	a, err := x.Directory.ActorByID(ctx, id)
	if err != nil {
		return "?"
	}
	return "@" + a.Handle
}

// place is the channel part of a header: "#name", or "DM @a ↔ @b".
func (x *Indexer) place(ctx context.Context, ch conversation.Channel) string {
	if ch.Kind != conversation.KindDM {
		return "#" + ch.Name
	}
	members, err := x.Conv.MemberIDs(ctx, ch.ID)
	if err != nil || len(members) == 0 {
		return "DM"
	}
	var hs []string
	for _, m := range members {
		hs = append(hs, x.handle(ctx, m))
	}
	sort.Strings(hs)
	return "DM " + strings.Join(hs, " ↔ ")
}

func (x *Indexer) threadTitle(ctx context.Context, rootID ids.ID) string {
	if rootID == ids.Nil {
		return ""
	}
	root, err := x.Conv.MessageByID(ctx, rootID)
	if err != nil || root.DeletedAt != nil {
		return ""
	}
	return FirstLine(root.Body, 80)
}

func date(t time.Time) string { return t.UTC().Format("2006-01-02") }

// chunks cuts body into chunks under header, leaving out the ones that
// look like they carry a secret; parts are numbered without gaps.
func (x *Indexer) chunks(base Chunk, header, body string) []Chunk {
	var out []Chunk
	for _, piece := range SplitText(body) {
		if rule := secretscan.Find(header + "\n" + piece); rule != "" {
			x.log().Info("search: chunk left out of the index", "source_kind", base.SourceKind, "source_id", base.SourceID, "rule", rule)
			continue
		}
		c := base
		c.ID = x.IDs.New()
		c.Part = len(out)
		c.Header = header
		c.Body = piece
		c.ContentHash = ContentHash(header, piece)
		c.IndexedAt = x.Clock.Now()
		out = append(out, c)
	}
	return out
}

func (x *Indexer) replace(ctx context.Context, kind SourceKind, sourceID ids.ID, cs []Chunk) error {
	if err := x.Store.ReplaceSource(ctx, kind, sourceID, cs); err != nil {
		return err
	}
	if len(cs) > 0 && x.Indexed != nil {
		x.Indexed()
	}
	return nil
}

func (x *Indexer) indexMessage(ctx context.Context, id ids.ID) error {
	m, err := x.Conv.MessageByID(ctx, id)
	if missing(err) || id == ids.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	if m.DeletedAt != nil || m.Kind != "text" || strings.TrimSpace(m.Body) == "" {
		return x.Store.DeleteSource(ctx, SourceMessage, m.ID)
	}
	ch, err := x.Conv.ChannelByID(ctx, m.ChannelID)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	parts := []string{x.place(ctx, ch)}
	if t := x.threadTitle(ctx, m.ThreadRootID); t != "" {
		parts = append(parts, "szál: „"+t+"”")
	}
	parts = append(parts, x.handle(ctx, m.AuthorID), date(m.CreatedAt))
	base := Chunk{WorkspaceID: m.WorkspaceID, SourceKind: SourceMessage, SourceID: m.ID, Visibility: VisibilityChannel,
		ChannelID: m.ChannelID, ThreadRootID: m.ThreadRootID, AuthorID: m.AuthorID, SourceAt: m.CreatedAt}
	return x.replace(ctx, SourceMessage, m.ID, x.chunks(base, strings.Join(parts, " · "), m.Body))
}

func (x *Indexer) indexWiki(ctx context.Context, workspaceID ids.ID, slug string) error {
	// The decisions page is indexed decision by decision (decision.recorded).
	if slug == decision.WikiSlug || slug == "" {
		return nil
	}
	p, err := x.Wiki.PageForIndex(ctx, workspaceID, slug)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.DeletedAt != nil {
		return x.Store.DeleteSource(ctx, SourceWiki, p.ID)
	}
	at := p.UpdatedAt
	if at.IsZero() {
		at = p.CreatedAt
	}
	base := Chunk{WorkspaceID: p.WorkspaceID, SourceKind: SourceWiki, SourceID: p.ID, SourceRef: p.Slug, Visibility: VisibilityWorkspace,
		AuthorID: p.CreatedBy, SourceAt: at}
	return x.replace(ctx, SourceWiki, p.ID, x.chunks(base, "Wiki: "+p.Title, p.Body))
}

// IndexDecision indexes one decision (the event handler and the backfill).
func (x *Indexer) IndexDecision(ctx context.Context, d decision.Decision) error {
	if d.ID == ids.Nil {
		return nil
	}
	header := fmt.Sprintf("Döntés %s", d.FormattedNumber())
	if ch, err := x.Conv.ChannelByID(ctx, d.ChannelID); err == nil {
		header += " · " + x.place(ctx, ch)
	}
	header += " · " + x.handle(ctx, d.AuthorID) + " · " + date(d.CreatedAt)
	body := fmt.Sprintf("%s: %s — Indoklás: %s", d.FormattedNumber(), d.Text, d.Rationale)
	if d.Supersedes != nil {
		body += fmt.Sprintf(" (felülírja: D-%d)", *d.Supersedes)
	}
	base := Chunk{WorkspaceID: d.WorkspaceID, SourceKind: SourceDecision, SourceID: d.ID, SourceRef: d.FormattedNumber(), Visibility: VisibilityWorkspace,
		ChannelID: d.ChannelID, AuthorID: d.AuthorID, SourceAt: d.CreatedAt}
	if d.ThreadRootID != nil {
		base.ThreadRootID = *d.ThreadRootID
	}
	return x.replace(ctx, SourceDecision, d.ID, x.chunks(base, header, body))
}

func (x *Indexer) indexSummary(ctx context.Context, workspaceID ids.ID, p compaction.CompactionDone, at time.Time) error {
	compID, fileID := parseID(p.CompactionID), parseID(p.SummaryFileID)
	chID, rootID := parseID(p.ChannelID), parseID(p.ThreadRootID)
	if compID == ids.Nil || fileID == ids.Nil || chID == ids.Nil || x.Summaries == nil {
		return nil
	}
	ch, err := x.Conv.ChannelByID(ctx, chID)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	text, err := x.Summaries(ctx, fileID)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// The newest summary of a unit (a regeneration or an edit is a new
	// compaction) replaces the older ones.
	if err := x.Store.DeleteSummaries(ctx, chID, rootID); err != nil {
		return err
	}
	parts := []string{"Összefoglaló", x.place(ctx, ch)}
	if t := x.threadTitle(ctx, rootID); t != "" {
		parts = append(parts, "szál: „"+t+"”")
	}
	parts = append(parts, date(at))
	base := Chunk{WorkspaceID: workspaceID, SourceKind: SourceSummary, SourceID: compID, Visibility: VisibilityChannel,
		ChannelID: chID, ThreadRootID: rootID, SourceAt: at}
	return x.replace(ctx, SourceSummary, compID, x.chunks(base, strings.Join(parts, " · "), text))
}

// BackfillDecisions indexes every decision of a workspace; returns how many.
func (x *Indexer) BackfillDecisions(ctx context.Context, workspaceID ids.ID) (int, error) {
	ds, err := x.Decisions.AllForIndex(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	for _, d := range ds {
		if err := x.IndexDecision(ctx, d); err != nil {
			return 0, err
		}
	}
	return len(ds), nil
}
