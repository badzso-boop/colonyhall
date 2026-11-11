// Package postgres implements git.Store on PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/git"
	"colonyhall/internal/modules/git/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements git.Store on Postgres.
type Store struct {
	DB *db.Postgres
}

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(s.DB.Q(ctx))
}

var _ git.Store = (*Store)(nil)

func toLinkCard(workspaceID ids.ID, row sqlcgen.GetGitLinkRow) git.LinkCard {
	return git.LinkCard{
		WorkspaceID: workspaceID,
		URL:         row.Url,
		Kind:        git.LinkKind(row.Kind),
		Title:       row.Title,
		State:       row.State,
		Author:      row.Author,
		FetchedAt:   row.FetchedAt,
	}
}

func (s *Store) GetLink(ctx context.Context, workspaceID ids.ID, url string) (git.LinkCard, error) {
	row, err := s.q(ctx).GetGitLink(ctx, sqlcgen.GetGitLinkParams{WorkspaceID: workspaceID, Url: url})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return git.LinkCard{}, git.ErrLinkNotFound
		}
		return git.LinkCard{}, err
	}
	return toLinkCard(workspaceID, row), nil
}

func (s *Store) GetLinks(ctx context.Context, workspaceID ids.ID, urls []string) ([]git.LinkCard, error) {
	if len(urls) == 0 {
		return []git.LinkCard{}, nil
	}
	rows, err := s.q(ctx).GetGitLinks(ctx, sqlcgen.GetGitLinksParams{WorkspaceID: workspaceID, Urls: urls})
	if err != nil {
		return nil, err
	}
	out := make([]git.LinkCard, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLinkCard(workspaceID, sqlcgen.GetGitLinkRow(r)))
	}
	return out, nil
}

func (s *Store) SaveLink(ctx context.Context, card git.LinkCard) error {
	return s.q(ctx).UpsertGitLink(ctx, sqlcgen.UpsertGitLinkParams{
		WorkspaceID: card.WorkspaceID,
		Url:         card.URL,
		Kind:        string(card.Kind),
		Title:       card.Title,
		State:       card.State,
		Author:      card.Author,
		FetchedAt:   card.FetchedAt,
	})
}

func (s *Store) ListRepoChannels(ctx context.Context, workspaceID ids.ID) ([]git.RepoChannel, error) {
	rows, err := s.q(ctx).ListGitRepoChannels(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]git.RepoChannel, 0, len(rows))
	for _, r := range rows {
		out = append(out, git.RepoChannel{
			WorkspaceID: r.WorkspaceID,
			Repo:        r.Repo,
			ChannelID:   r.ChannelID,
			CreatedAt:   r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Store) GetRepoChannel(ctx context.Context, workspaceID ids.ID, repo string) (git.RepoChannel, error) {
	row, err := s.q(ctx).GetGitRepoChannel(ctx, sqlcgen.GetGitRepoChannelParams{
		WorkspaceID: workspaceID,
		Repo:        repo,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return git.RepoChannel{}, git.ErrMappingNotFound
		}
		return git.RepoChannel{}, err
	}
	return git.RepoChannel{
		WorkspaceID: row.WorkspaceID,
		Repo:        row.Repo,
		ChannelID:   row.ChannelID,
		CreatedAt:   row.CreatedAt,
	}, nil
}

func (s *Store) FindRepoChannels(ctx context.Context, repo string) ([]git.RepoChannel, error) {
	rows, err := s.q(ctx).GetGitRepoChannelsByRepo(ctx, repo)
	if err != nil {
		return nil, err
	}
	out := make([]git.RepoChannel, 0, len(rows))
	for _, r := range rows {
		out = append(out, git.RepoChannel{
			WorkspaceID: r.WorkspaceID,
			Repo:        r.Repo,
			ChannelID:   r.ChannelID,
			CreatedAt:   r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Store) SetRepoChannel(ctx context.Context, mapping git.RepoChannel) error {
	return s.q(ctx).UpsertGitRepoChannel(ctx, sqlcgen.UpsertGitRepoChannelParams{
		WorkspaceID: mapping.WorkspaceID,
		Repo:        mapping.Repo,
		ChannelID:   mapping.ChannelID,
		CreatedAt:   mapping.CreatedAt,
	})
}

func (s *Store) DeleteRepoChannel(ctx context.Context, workspaceID ids.ID, repo string) error {
	return s.q(ctx).DeleteGitRepoChannel(ctx, sqlcgen.DeleteGitRepoChannelParams{
		WorkspaceID: workspaceID,
		Repo:        repo,
	})
}

func (s *Store) RecordDelivery(ctx context.Context, deliveryID string, now time.Time) (bool, error) {
	return s.q(ctx).InsertWebhookDelivery(ctx, sqlcgen.InsertWebhookDeliveryParams{
		DeliveryID: deliveryID,
		ReceivedAt: now,
	})
}
