package git

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store defines the storage port for the git module.
type Store interface {
	// GetLink returns the workspace's cached link card for a URL, or ErrLinkNotFound.
	GetLink(ctx context.Context, workspaceID ids.ID, url string) (LinkCard, error)

	// GetLinks returns the workspace's cached link cards for the URLs.
	GetLinks(ctx context.Context, workspaceID ids.ID, urls []string) ([]LinkCard, error)

	// SaveLink inserts or updates a cached link card (keyed by its workspace and URL).
	SaveLink(ctx context.Context, card LinkCard) error

	// ListRepoChannels returns all repo-to-channel mappings for a workspace.
	ListRepoChannels(ctx context.Context, workspaceID ids.ID) ([]RepoChannel, error)

	// GetRepoChannel returns the mapping for a repo, or ErrMappingNotFound.
	GetRepoChannel(ctx context.Context, workspaceID ids.ID, repo string) (RepoChannel, error)

	// FindRepoChannels returns mappings for a repository across workspaces (used by webhooks).
	FindRepoChannels(ctx context.Context, repo string) ([]RepoChannel, error)

	// SetRepoChannel creates or updates a mapping.
	SetRepoChannel(ctx context.Context, mapping RepoChannel) error

	// DeleteRepoChannel removes a mapping.
	DeleteRepoChannel(ctx context.Context, workspaceID ids.ID, repo string) error

	// GetWorkspaceGitea returns the workspace's own Gitea, or ErrNoOwnGitea.
	GetWorkspaceGitea(ctx context.Context, workspaceID ids.ID) (WorkspaceGitea, error)
	// SetWorkspaceGitea stores (or replaces) the workspace's own Gitea.
	SetWorkspaceGitea(ctx context.Context, cfg WorkspaceGitea) error

	// RecordDelivery records a webhook delivery ID. Returns true if newly recorded, false if already seen.
	RecordDelivery(ctx context.Context, deliveryID string, now time.Time) (bool, error)
}
