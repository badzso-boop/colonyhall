// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package provider

import (
	"context"
	"errors"
	"net/http"
)

// ErrUnauthorizedWebhook is returned when the webhook signature verification fails.
var ErrUnauthorizedWebhook = errors.New("invalid webhook signature")

// ResourceKind indicates the type of Git resource.
type ResourceKind string

const (
	ResourceRepo   ResourceKind = "repo"
	ResourcePull   ResourceKind = "pull"
	ResourceIssue  ResourceKind = "issue"
	ResourceCommit ResourceKind = "commit"
)

// ResourceRef identifies a specific Git resource on a provider.
type ResourceRef struct {
	Kind  ResourceKind
	Owner string
	Repo  string
	ID    string // Issue/PR number or commit SHA
	URL   string
}

// ResourceCard holds metadata fetched for a resource.
type ResourceCard struct {
	URL    string
	Kind   ResourceKind
	Title  string
	State  string
	Author string
}

// WebhookEvent represents a parsed Git webhook event.
type WebhookEvent struct {
	DeliveryID string
	Repo       string // "owner/repo"
	Action     string
	Summary    string
}

// GitProvider is the port interface for external Git hosts (Gitea, GitHub, GitLab).
type GitProvider interface {
	// Name returns the provider identifier, e.g. "gitea".
	Name() string

	// ParseURL checks if rawURL belongs to this provider and parses it into a ResourceRef.
	// Returns false if the URL does not belong to this provider.
	ParseURL(rawURL string) (ResourceRef, bool)

	// ParseRef parses either a full URL or a shorthand like "owner/repo#12" or "owner/repo@sha".
	ParseRef(refStr string) (ResourceRef, bool)

	// FetchResource queries the provider's API for the resource's title, state, and author.
	FetchResource(ctx context.Context, ref ResourceRef) (ResourceCard, error)

	// ParseWebhook validates the webhook signature and parses event payload into a WebhookEvent.
	ParseWebhook(r *http.Request, secret string) (WebhookEvent, error)
}
