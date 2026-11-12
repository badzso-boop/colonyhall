// Package postgres implements search.Store on Postgres with full-text
// search (hungarian and simple configurations) and pgvector.
package postgres

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/modules/search"
	"colonyhall/internal/modules/search/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements search.Store.
type Store struct{ DB *db.Postgres }

var _ search.Store = (*Store)(nil)

// efSearch is the HNSW candidate list size for the vector leg.
const efSearch = 100

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func optID(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func fromOpt(u *uuid.UUID) ids.ID {
	if u == nil {
		return ids.Nil
	}
	return ids.ID(*u)
}

func (s *Store) ReplaceSource(ctx context.Context, kind search.SourceKind, sourceID ids.ID, chunks []search.Chunk) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		q := s.q(ctx)
		for _, c := range chunks {
			err := q.UpsertChunk(ctx, sqlcgen.UpsertChunkParams{
				ID: uuid.UUID(c.ID), WorkspaceID: uuid.UUID(c.WorkspaceID), SourceKind: string(kind),
				SourceID: uuid.UUID(sourceID), SourceRef: c.SourceRef, Part: int32(c.Part), Visibility: string(c.Visibility),
				ChannelID: optID(c.ChannelID), ThreadRootID: optID(c.ThreadRootID), AuthorID: optID(c.AuthorID),
				SourceAt: c.SourceAt, Header: c.Header, Body: c.Body, ContentHash: c.ContentHash, IndexedAt: c.IndexedAt,
			})
			if err != nil {
				return err
			}
		}
		return q.DeleteSourcePartsFrom(ctx, sqlcgen.DeleteSourcePartsFromParams{
			SourceKind: string(kind), SourceID: uuid.UUID(sourceID), FromPart: int32(len(chunks)),
		})
	})
}

func (s *Store) DeleteSource(ctx context.Context, kind search.SourceKind, sourceID ids.ID) error {
	return s.q(ctx).DeleteSourcePartsFrom(ctx, sqlcgen.DeleteSourcePartsFromParams{
		SourceKind: string(kind), SourceID: uuid.UUID(sourceID), FromPart: 0,
	})
}

func (s *Store) DeleteSummaries(ctx context.Context, channelID, threadRootID ids.ID) error {
	return s.q(ctx).DeleteSummaries(ctx, sqlcgen.DeleteSummariesParams{ChannelID: optID(channelID), ThreadRootID: optID(threadRootID)})
}

func (s *Store) PendingEmbeddings(ctx context.Context, now time.Time, limit int) ([]search.PendingChunk, error) {
	rows, err := s.q(ctx).PendingEmbeddings(ctx, sqlcgen.PendingEmbeddingsParams{Now: &now, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]search.PendingChunk, len(rows))
	for i, r := range rows {
		out[i] = search.PendingChunk{ID: ids.ID(r.ID), Header: r.Header, Body: r.Body}
	}
	return out, nil
}

// vectorText is pgvector's text form: [0.1,0.2,…].
func vectorText(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 10)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (s *Store) SetEmbedding(ctx context.Context, id ids.ID, vec []float32, model string, at time.Time) error {
	return s.q(ctx).SetEmbedding(ctx, sqlcgen.SetEmbeddingParams{Embedding: vectorText(vec), EmbedModel: &model, EmbeddedAt: &at, ID: uuid.UUID(id)})
}

func (s *Store) EmbedFailed(ctx context.Context, chunkIDs []ids.ID, retryAt time.Time) error {
	us := make([]uuid.UUID, len(chunkIDs))
	for i, id := range chunkIDs {
		us[i] = uuid.UUID(id)
	}
	return s.q(ctx).EmbedFailed(ctx, sqlcgen.EmbedFailedParams{RetryAt: &retryAt, Ids: us})
}

func (s *Store) EmbedModels(ctx context.Context) ([]string, error) {
	return s.q(ctx).EmbedModels(ctx)
}

func (s *Store) ResetEmbeddings(ctx context.Context, keepModel string) (int, error) {
	n, err := s.q(ctx).ResetEmbeddings(ctx, &keepModel)
	return int(n), err
}

func uuids(xs []ids.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(xs))
	for i, x := range xs {
		out[i] = uuid.UUID(x)
	}
	return out
}

func kinds(xs []search.SourceKind) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = string(x)
	}
	return out
}

func chunk(id, ws uuid.UUID, kind string, src uuid.UUID, ref string, part int32, vis string, ch, root, author *uuid.UUID,
	at time.Time, header, body, hash, model string, indexed time.Time) search.Chunk {
	return search.Chunk{
		ID: ids.ID(id), WorkspaceID: ids.ID(ws), SourceKind: search.SourceKind(kind), SourceID: ids.ID(src),
		SourceRef: ref, Part: int(part), Visibility: search.Visibility(vis), ChannelID: fromOpt(ch), ThreadRootID: fromOpt(root),
		AuthorID: fromOpt(author), SourceAt: at, Header: header, Body: body, ContentHash: hash, EmbedModel: model,
		IndexedAt: indexed,
	}
}

func (s *Store) Keyword(ctx context.Context, acc search.Access, query string, f search.Filters, limit int) ([]search.Candidate, error) {
	rows, err := s.q(ctx).KeywordSearch(ctx, sqlcgen.KeywordSearchParams{
		WorkspaceID: uuid.UUID(acc.WorkspaceID), ChannelIds: uuids(acc.ChannelIDs), Kinds: kinds(f.Kinds),
		ChannelID: optID(f.ChannelID), AuthorID: optID(f.AuthorID), Since: f.Since, Until: f.Until,
		Lim: int32(limit), Query: query,
	})
	if err != nil {
		return nil, err
	}
	out := make([]search.Candidate, len(rows))
	for i, r := range rows {
		out[i] = search.Candidate{
			Chunk: chunk(r.ID, r.WorkspaceID, r.SourceKind, r.SourceID, r.SourceRef, r.Part, r.Visibility, r.ChannelID, r.ThreadRootID,
				r.AuthorID, r.SourceAt, r.Header, r.Body, r.ContentHash, r.EmbedModel, r.IndexedAt),
			Score: r.Score, Rank: i + 1, Snippet: r.Snippet,
		}
	}
	return out, nil
}

func (s *Store) Semantic(ctx context.Context, acc search.Access, vec []float32, model string, f search.Filters, limit int) ([]search.Candidate, error) {
	var out []search.Candidate
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		// The access rule is part of the WHERE clause, so a filtered HNSW
		// scan can only return fewer rows, never foreign ones; the
		// iterative scan keeps it from returning too few.
		if _, err := s.DB.Q(ctx).Exec(ctx, "SET LOCAL hnsw.iterative_scan = relaxed_order"); err != nil {
			return err
		}
		if _, err := s.DB.Q(ctx).Exec(ctx, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(efSearch)); err != nil {
			return err
		}
		rows, err := s.q(ctx).SemanticSearch(ctx, sqlcgen.SemanticSearchParams{
			Embedding: vectorText(vec), WorkspaceID: uuid.UUID(acc.WorkspaceID), ChannelIds: uuids(acc.ChannelIDs),
			EmbedModel: &model, Kinds: kinds(f.Kinds), ChannelID: optID(f.ChannelID), AuthorID: optID(f.AuthorID),
			Since: f.Since, Until: f.Until, Lim: int32(limit),
		})
		if err != nil {
			return err
		}
		out = make([]search.Candidate, len(rows))
		for i, r := range rows {
			out[i] = search.Candidate{
				Chunk: chunk(r.ID, r.WorkspaceID, r.SourceKind, r.SourceID, r.SourceRef, r.Part, r.Visibility, r.ChannelID, r.ThreadRootID,
					r.AuthorID, r.SourceAt, r.Header, r.Body, r.ContentHash, r.EmbedModel, r.IndexedAt),
				Score: r.Score,
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// relaxed_order may return rows slightly out of order.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}

func (s *Store) InsertLog(ctx context.Context, e search.LogEntry) error {
	timings, err := json.Marshal(e.TimingsMS)
	if err != nil {
		return err
	}
	refs, degraded := e.ResultRefs, e.Degraded
	if refs == nil {
		refs = []string{}
	}
	if degraded == nil {
		degraded = []string{}
	}
	return s.q(ctx).InsertLog(ctx, sqlcgen.InsertLogParams{
		ID: uuid.UUID(e.ID), WorkspaceID: uuid.UUID(e.WorkspaceID), ActorID: uuid.UUID(e.ActorID), At: e.At,
		Query: e.Query, Mode: string(e.Mode), ResultCount: int32(e.ResultCount), ResultRefs: refs,
		TimingsMs: timings, Degraded: degraded,
	})
}

func (s *Store) PurgeLogs(ctx context.Context, before time.Time) (int, error) {
	n, err := s.q(ctx).PurgeLogs(ctx, before)
	return int(n), err
}

func (s *Store) Stats(ctx context.Context, workspaceID ids.ID) (search.Stats, error) {
	r, err := s.q(ctx).Stats(ctx, uuid.UUID(workspaceID))
	if err != nil {
		return search.Stats{}, err
	}
	st := search.Stats{Chunks: int(r.Chunks), Pending: int(r.Pending), Models: r.Models}
	if r.LastEmbedAt.After(time.Unix(0, 0)) {
		t := r.LastEmbedAt
		st.LastEmbedAt = &t
	}
	return st, nil
}
