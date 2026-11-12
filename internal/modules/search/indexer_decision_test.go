package search_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/search"
	"colonyhall/internal/platform/ids"
)

// capturingStore keeps the chunks the indexer writes.
type capturingStore struct {
	search.Store
	chunks []search.Chunk
}

func (c *capturingStore) ReplaceSource(_ context.Context, _ search.SourceKind, _ ids.ID, cs []search.Chunk) error {
	c.chunks = append(c.chunks, cs...)
	return nil
}

type dmChannel struct {
	oneChannel
	members []ids.ID
}

func (d dmChannel) MemberIDs(context.Context, ids.ID) ([]ids.ID, error) { return d.members, nil }

type handles map[ids.ID]string

func (h handles) ActorByID(_ context.Context, id ids.ID) (identity.Actor, error) {
	return identity.Actor{ID: id, Handle: h[id]}, nil
}

// A decision is searchable by the whole workspace, so its header must not
// name a private channel or a DM's participants (push security review,
// 2026-11-12); a public channel's name stays.
func TestDecisionHeaderHidesNonPublicPlaces(t *testing.T) {
	gen := &ids.Sequential{}
	ws, anna, bela := gen.New(), gen.New(), gen.New()
	dir := handles{anna: "anna", bela: "bela"}
	for _, tc := range []struct {
		ch      conversation.Channel
		want    string
		mustNot []string
	}{
		{conversation.Channel{ID: gen.New(), WorkspaceID: ws, Kind: conversation.KindPublic, Name: "general"}, "#general", nil},
		{conversation.Channel{ID: gen.New(), WorkspaceID: ws, Kind: conversation.KindPrivate, Name: "secret-merger"}, "", []string{"secret-merger"}},
		{conversation.Channel{ID: gen.New(), WorkspaceID: ws, Kind: conversation.KindDM, Name: "dm"}, "", []string{"DM", "@bela"}},
	} {
		store := &capturingStore{}
		x := &search.Indexer{Store: store, Conv: dmChannel{oneChannel{ch: tc.ch}, []ids.ID{anna, bela}}, Directory: dir,
			IDs: gen, Clock: fixedClock{time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}}
		d := decision.Decision{ID: gen.New(), Number: 7, WorkspaceID: ws, AuthorID: anna, ChannelID: tc.ch.ID,
			Text: "We use Postgres", Rationale: "one database", CreatedAt: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
		if err := x.IndexDecision(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		if len(store.chunks) == 0 {
			t.Fatalf("%s: nothing indexed", tc.ch.Kind)
		}
		for _, c := range store.chunks {
			if c.Visibility != search.VisibilityWorkspace {
				t.Fatalf("%s: visibility %s", tc.ch.Kind, c.Visibility)
			}
			if tc.want != "" && !strings.Contains(c.Header, tc.want) {
				t.Errorf("%s: header %q lacks %q", tc.ch.Kind, c.Header, tc.want)
			}
			for _, bad := range tc.mustNot {
				if strings.Contains(c.Header, bad) || strings.Contains(c.Body, bad) {
					t.Errorf("%s: %q leaks into %q / %q", tc.ch.Kind, bad, c.Header, c.Body)
				}
			}
		}
	}
}
