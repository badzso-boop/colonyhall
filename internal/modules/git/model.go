// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package git

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/modules/git/provider"
	"colonyhall/internal/platform/ids"
)

// Standard errors for the git module.
var (
	ErrInvalidRepo         = errors.New("invalid repository name; expected owner/repo")
	ErrMappingNotFound     = errors.New("repository mapping not found")
	ErrInvalidWebhook      = errors.New("invalid webhook payload")
	ErrUnauthorizedWebhook = provider.ErrUnauthorizedWebhook
	ErrDisabled            = errors.New("git integration is not configured")
	ErrLinkNotFound        = errors.New("git link not found")
	// ErrNoOwnGitea: the workspace has no Gitea of its own (it uses the shared one).
	ErrNoOwnGitea = errors.New("the workspace has no own Gitea")
)

// LinkKind indicates the type of Git resource.
type LinkKind string

const (
	KindRepo   LinkKind = "repo"
	KindPull   LinkKind = "pull"
	KindIssue  LinkKind = "issue"
	KindCommit LinkKind = "commit"
)

// LinkCard holds unfurled metadata for a git URL.
type LinkCard struct {
	// WorkspaceID: the cache is per workspace (round 45), so a private
	// repository's title never shows in another customer's workspace.
	WorkspaceID ids.ID    `json:"-"`
	URL         string    `json:"url"`
	Kind        LinkKind  `json:"kind"`
	Title       string    `json:"title"`
	State       string    `json:"state"`
	Author      string    `json:"author,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// RepoChannel maps a Git repository to a chat channel within a workspace.
type RepoChannel struct {
	WorkspaceID ids.ID    `json:"workspace_id"`
	Repo        string    `json:"repo"`
	ChannelID   ids.ID    `json:"channel_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// ValidateRepo ensures a repository name is formatted as owner/repo without traversal characters.
func ValidateRepo(repo string) error {
	repo = strings.TrimSpace(repo)
	if repo == "" || len(repo) > 150 {
		return ErrInvalidRepo
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrInvalidRepo
	}
	if strings.ContainsAny(repo, " \t\r\n\\:?*\"'<>|") {
		return fmt.Errorf("%w: contains invalid characters", ErrInvalidRepo)
	}
	return nil
}

// WorkspaceGitea is a workspace's own Gitea (round 45), used instead of the
// server's shared one while Enabled. Token and WebhookSecret are secrets:
// never in an API response, a log or an event.
type WorkspaceGitea struct {
	WorkspaceID   ids.ID
	Enabled       bool
	URL           string // the API address the server calls
	PublicURL     string // the address people use in links, when different
	Token         string // read-only API token
	WebhookSecret string
	UpdatedAt     time.Time
}
