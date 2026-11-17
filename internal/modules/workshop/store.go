// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists the Workshop. Every call runs in the workspace's scope;
// "not found" is db.ErrNotFound.
type Store interface {
	InsertConnector(ctx context.Context, c Connector) error
	// UpdateConnector writes title, base URL, auth, header, secret,
	// approval_off and the risk overrides.
	UpdateConnector(ctx context.Context, c Connector) error
	SetConnectorTools(ctx context.Context, workspaceID, id ids.ID, tools []ToolInfo, at time.Time, state, lastError string) error
	GetConnector(ctx context.Context, workspaceID, id ids.ID) (Connector, error)
	ListConnectors(ctx context.Context, workspaceID ids.ID) ([]Connector, error)
	DeleteConnector(ctx context.Context, workspaceID, id ids.ID, at time.Time) error

	InsertGrant(ctx context.Context, g Grant) error
	RevokeGrant(ctx context.Context, workspaceID, id ids.ID, at time.Time) error
	RevokeGrantsForTarget(ctx context.Context, workspaceID ids.ID, kind string, targetID ids.ID, at time.Time) error
	ListGrants(ctx context.Context, workspaceID ids.ID) ([]Grant, error)
	GrantsForAgent(ctx context.Context, workspaceID, agentID ids.ID) ([]Grant, error)

	InsertToolCall(ctx context.Context, c ToolCall) error
	// ListToolCalls returns the newest calls first, before the given time
	// (nil: from now), optionally of one target.
	ListToolCalls(ctx context.Context, workspaceID ids.ID, targetID *ids.ID, before *time.Time, limit int) ([]ToolCall, error)
	// CountCallsSince counts the calls that ran (not the denied ones) since t.
	CountCallsSince(ctx context.Context, workspaceID ids.ID, since time.Time) (int, error)

	InsertExtension(ctx context.Context, e Extension) error
	// UpdateExtension writes title, description, state, live version,
	// approval_off and the disabled reason.
	UpdateExtension(ctx context.Context, e Extension) error
	SetExtensionErrors(ctx context.Context, workspaceID, id ids.ID, consecutive int, recent []time.Time) error
	GetExtension(ctx context.Context, workspaceID, id ids.ID) (Extension, error)
	ListExtensions(ctx context.Context, workspaceID ids.ID) ([]Extension, error)
	DeleteExtension(ctx context.Context, workspaceID, id ids.ID, at time.Time) error

	InsertVersion(ctx context.Context, v Version) error
	GetVersion(ctx context.Context, workspaceID, extensionID ids.ID, version int) (Version, error)
	// ListVersions returns the versions newest first, without source and bundle.
	ListVersions(ctx context.Context, workspaceID, extensionID ids.ID) ([]Version, error)
	InsertApproval(ctx context.Context, a Approval) error
	ListApprovals(ctx context.Context, workspaceID, extensionID ids.ID) ([]Approval, error)

	GetDoc(ctx context.Context, workspaceID, extensionID ids.ID, collection, docID string) (StoreDoc, error)
	PutDoc(ctx context.Context, d StoreDoc) error
	DeleteDoc(ctx context.Context, workspaceID, extensionID ids.ID, collection, docID string) error
	// ListDocs returns a collection's documents by id, after the given id.
	ListDocs(ctx context.Context, workspaceID, extensionID ids.ID, collection, after string, limit int) ([]StoreDoc, error)
	// StoreUsage returns the number of documents and their bytes.
	StoreUsage(ctx context.Context, workspaceID, extensionID ids.ID) (int, int, error)

	// Routines (round 56c).
	UpsertRoutineState(ctx context.Context, st RoutineState) error
	ListRoutineStates(ctx context.Context, workspaceID, extensionID ids.ID) ([]RoutineState, error)
	GetRoutineState(ctx context.Context, workspaceID, id ids.ID) (RoutineState, error)
	SetRoutineNext(ctx context.Context, workspaceID, id ids.ID, next, last *time.Time) error
	SetRoutineWebhook(ctx context.Context, workspaceID, id ids.ID, hash []byte) error
	// DueRoutines lists the routines due across workspaces (system scope).
	DueRoutines(ctx context.Context, now time.Time, limit int) ([]RoutineState, error)
	InsertRoutineRun(ctx context.Context, r RoutineRun) error
	// ClaimRoutineRuns marks pending runs running and returns them
	// (system scope; each once).
	ClaimRoutineRuns(ctx context.Context, now time.Time, limit int) ([]RoutineRun, error)
	FinishRoutineRun(ctx context.Context, workspaceID, id ids.ID, state, errText string, at time.Time) error
	ListRoutineRuns(ctx context.Context, workspaceID, extensionID ids.ID, limit int) ([]RoutineRun, error)
}
