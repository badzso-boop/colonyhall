package git

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store defines the storage port for the git module.
type Store interface {
	// GetLink returns the cached link card for a URL, or ErrLinkNotFound.
	GetLink(ctx context.Context, url string) (LinkCard, error)

	// GetLinks returns cached link cards for the specified URLs.
	GetLinks(ctx context.Context, urls []string) ([]LinkCard, error)

	// SaveLink inserts or updates a cached link card.
	SaveLink(ctx context.Context, card LinkCard) error

	// ListRepoChannels returns all repo-to-channel mappings for a workspace.
	ListRepoChannels(ctx context.Context, workspaceID ids.ID) ([]RepoChannel, error)

	// GetRepoChannel returns the mapping for a repo, or ErrMappingNotFound.
	GetRepoChannel(ctx context.Context, workspaceID ids.ID, repo string) (RepoChannel, error)

	// SetRepoChannel creates or updates a mapping.
	SetRepoChannel(ctx context.Context, mapping RepoChannel) error

	// DeleteRepoChannel removes a mapping.
	DeleteRepoChannel(ctx context.Context, workspaceID ids.ID, repo string) error

	// RecordDelivery records a webhook delivery ID. Returns true if newly recorded, false if already seen.
	RecordDelivery(ctx context.Context, deliveryID string, now time.Time) (bool, error)
}
