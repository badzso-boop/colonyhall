// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/extrunner"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// FetchRequest is an extension's outbound request. Auth holds the
// connector's authentication headers (they go to the URL's own origin
// only); Hosts, when set, limits the request to those host names.
type FetchRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Auth    map[string]string
	Body    []byte
	Hosts   []string
}

// FetchResponse is the answer.
type FetchResponse struct {
	Status      int
	ContentType string
	Body        []byte
}

// Fetcher does the extensions' outbound HTTP (adapter: mcpremote.Fetcher).
type Fetcher interface {
	Fetch(ctx context.Context, r FetchRequest) (FetchResponse, error)
}

// Compiler turns a tool's TypeScript or JavaScript into the runner's
// script (adapter: compile.Esbuild).
type Compiler interface {
	Compile(source string) (string, error)
}

// Bots creates and archives the extensions' bots (the identity module).
type Bots interface {
	CreateAgentActor(ctx context.Context, by identity.Actor, in identity.AgentActorInput) (identity.Actor, string, error)
	ArchiveAgent(ctx context.Context, by identity.Actor, agentID ids.ID) (identity.Actor, error)
}

// CoreTool calls one of Colonyhall's own MCP tools as the actor (the
// server wires it to an in-memory MCP session, so every check of the
// tool applies).
type CoreTool func(ctx context.Context, as identity.Actor, tool string, input json.RawMessage) (string, error)

// CallBudget returns the workspace's monthly call budget (nil: unlimited).
type CallBudget func(ctx context.Context, workspaceID ids.ID) (*int, error)

// ExtensionInput creates an extension.
type ExtensionInput struct {
	Slug        string
	Title       string
	Description string
	// BotHandle names the extension's bot (decision 2026-11-17); default
	// x-<slug>. BotName is its display name (default: the title).
	BotHandle string
	BotName   string
}

// CreateExtension adds an extension (a draft) and its bot. Owner and
// admins; the creator is its maintainer.
func (s *Service) CreateExtension(ctx context.Context, by identity.Actor, in ExtensionInput) (Extension, error) {
	if err := admin(by); err != nil {
		return Extension{}, err
	}
	if !ValidSlug(in.Slug) {
		return Extension{}, ErrBadSlug
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 80 {
		return Extension{}, ErrBadTitle
	}
	if utf8.RuneCountInString(in.Description) > 2000 {
		return Extension{}, fmt.Errorf("%w: the description is longer than 2000 characters", ErrBadManifest)
	}
	handle := strings.TrimPrefix(strings.TrimSpace(in.BotHandle), "@")
	if handle == "" {
		handle = "x-" + strings.ReplaceAll(in.Slug, "_", "-")
	}
	name := strings.TrimSpace(in.BotName)
	if name == "" {
		name = in.Title
	}
	now := s.Clock.Now()
	e := Extension{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, Slug: in.Slug, Title: in.Title, Description: in.Description,
		MaintainerID: by.ID, State: StateDraft, CreatedAt: now, UpdatedAt: now}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		bot, _, err := s.Bots.CreateAgentActor(ctx, by, identity.AgentActorInput{Handle: handle, DisplayName: name, OwnerID: by.ID,
			Description: "Workshop extension: " + in.Title, AvatarEmoji: "🧩"})
		if err != nil {
			return err
		}
		e.BotID = bot.ID
		if err := s.Store.InsertExtension(ctx, e); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrSlugTaken
			}
			return err
		}
		seq, err = s.emit(ctx, by, EventExtensionCreated, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug,
			BotID: bot.ID.String()})
		return err
	})
	if err != nil {
		return Extension{}, err
	}
	s.notify(seq)
	return e, nil
}

func (s *Service) extension(ctx context.Context, by identity.Actor, id ids.ID) (Extension, error) {
	e, err := s.Store.GetExtension(ctx, by.WorkspaceID, id)
	return e, notFound(err)
}

// VersionInput is a new version: the manifest and, for code tools, the
// TypeScript or JavaScript source.
type VersionInput struct {
	Manifest Manifest
	Source   string
}

// AddVersion stores a new version (it does not go live until approved).
// The by actor is a person (owner or admin) or, from 56d, the Builder.
func (s *Service) AddVersion(ctx context.Context, by identity.Actor, id ids.ID, in VersionInput) (Version, Diff, error) {
	if err := admin(by); err != nil {
		return Version{}, Diff{}, err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return Version{}, Diff{}, err
	}
	if len(in.Source) > MaxSource {
		return Version{}, Diff{}, fmt.Errorf("%w: the source is larger than 256 KB", ErrBadManifest)
	}
	cs, err := s.Store.ListConnectors(ctx, by.WorkspaceID)
	if err != nil {
		return Version{}, Diff{}, err
	}
	conns := map[string]Connector{}
	for _, c := range cs {
		conns[c.Slug] = c
	}
	m := in.Manifest
	if err := m.normalize(conns); err != nil {
		return Version{}, Diff{}, err
	}
	v := Version{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, ExtensionID: e.ID, Manifest: m, CreatedBy: by.ID, CreatedAt: s.Clock.Now()}
	if m.HasJS() {
		if strings.TrimSpace(in.Source) == "" {
			return Version{}, Diff{}, fmt.Errorf("%w: a js tool needs the source", ErrBadManifest)
		}
		if s.Compiler == nil {
			return Version{}, Diff{}, ErrCompile
		}
		bundle, err := s.Compiler.Compile(in.Source)
		if err != nil {
			return Version{}, Diff{}, fmt.Errorf("%w: %s", ErrCompile, err.Error())
		}
		v.Source, v.Bundle = in.Source, bundle
	}
	sum := sha256.Sum256([]byte(v.Bundle + "\x00" + manifestKey(m)))
	v.BundleSHA256 = hex.EncodeToString(sum[:])
	var diff Diff
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		vs, err := s.Store.ListVersions(ctx, by.WorkspaceID, e.ID)
		if err != nil {
			return err
		}
		v.Version = 1
		if len(vs) > 0 {
			v.Version = vs[0].Version + 1
		}
		if err := s.Store.InsertVersion(ctx, v); err != nil {
			return err
		}
		live, err := s.liveVersion(ctx, e)
		if err != nil {
			return err
		}
		diff = DiffVersions(live, v)
		seq, err = s.emit(ctx, by, EventVersionAdded, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug, Version: v.Version})
		return err
	})
	if err != nil {
		return Version{}, Diff{}, err
	}
	s.notify(seq)
	return v, diff, nil
}

func manifestKey(m Manifest) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func (s *Service) liveVersion(ctx context.Context, e Extension) (*Version, error) {
	if e.LiveVersion == 0 {
		return nil, nil
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ApproveVersion makes a version live after a person saw what it adds
// (the diff is recorded). Owner and admins.
func (s *Service) ApproveVersion(ctx context.Context, by identity.Actor, id ids.ID, version int) (Extension, Diff, error) {
	if err := admin(by); err != nil {
		return Extension{}, Diff{}, err
	}
	var e Extension
	var diff Diff
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if e, err = s.extension(ctx, by, id); err != nil {
			return err
		}
		v, err := s.Store.GetVersion(ctx, by.WorkspaceID, e.ID, version)
		if err != nil {
			return ErrVersionNotFound
		}
		live, err := s.liveVersion(ctx, e)
		if err != nil {
			return err
		}
		diff = DiffVersions(live, v)
		now := s.Clock.Now()
		if err := s.Store.InsertApproval(ctx, Approval{ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, ExtensionID: e.ID,
			Version: version, DecidedBy: by.ID, Diff: diff, CreatedAt: now}); err != nil {
			return err
		}
		e.LiveVersion, e.State, e.DisabledReason, e.UpdatedAt = version, StateLive, "", now
		// The owner switched the approval off for what was live then; a
		// version that adds anything (tools, risks, connectors, hosts, code)
		// approved by someone else gets the approval back (security review).
		if e.ApprovalOff && by.Role != identity.RoleOwner && !diffEmpty(diff) {
			e.ApprovalOff = false
		}
		if err := s.syncRoutines(ctx, by, e, v); err != nil {
			return err
		}
		if err := s.Store.UpdateExtension(ctx, e); err != nil {
			return err
		}
		if err := s.Store.SetExtensionErrors(ctx, e.WorkspaceID, e.ID, 0, nil); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventVersionApproved, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug,
			Version: version, Diff: &diff})
		return err
	})
	if err != nil {
		return Extension{}, Diff{}, err
	}
	s.notify(seq)
	return e, diff, nil
}

// Rollback makes an earlier approved version live again.
func (s *Service) Rollback(ctx context.Context, by identity.Actor, id ids.ID, version int) (Extension, error) {
	if err := admin(by); err != nil {
		return Extension{}, err
	}
	var e Extension
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if e, err = s.extension(ctx, by, id); err != nil {
			return err
		}
		as, err := s.Store.ListApprovals(ctx, by.WorkspaceID, e.ID)
		if err != nil {
			return err
		}
		ok := false
		for _, a := range as {
			ok = ok || a.Version == version
		}
		if !ok {
			return ErrNotApproved
		}
		if e.ApprovalOff && by.Role != identity.RoleOwner && e.LiveVersion != version {
			e.ApprovalOff = false // see ApproveVersion
		}
		v, err := s.Store.GetVersion(ctx, by.WorkspaceID, e.ID, version)
		if err != nil {
			return ErrVersionNotFound
		}
		if err := s.syncRoutines(ctx, by, e, v); err != nil {
			return err
		}
		e.LiveVersion, e.State, e.DisabledReason, e.UpdatedAt = version, StateLive, "", s.Clock.Now()
		if err := s.Store.UpdateExtension(ctx, e); err != nil {
			return err
		}
		if err := s.Store.SetExtensionErrors(ctx, e.WorkspaceID, e.ID, 0, nil); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventRolledBack, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug, Version: version})
		return err
	})
	if err != nil {
		return Extension{}, err
	}
	s.notify(seq)
	return e, nil
}

// SetEnabled switches an extension off or on again (on: its live version).
func (s *Service) SetEnabled(ctx context.Context, by identity.Actor, id ids.ID, enabled bool) (Extension, error) {
	if err := admin(by); err != nil {
		return Extension{}, err
	}
	var e Extension
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if e, err = s.extension(ctx, by, id); err != nil {
			return err
		}
		if enabled {
			if e.LiveVersion == 0 {
				return ErrNoLiveVersion
			}
			e.State, e.DisabledReason = StateLive, ""
			if err := s.Store.SetExtensionErrors(ctx, e.WorkspaceID, e.ID, 0, nil); err != nil {
				return err
			}
		} else {
			e.State, e.DisabledReason = StateDisabled, "@"+by.Handle
		}
		e.UpdatedAt = s.Clock.Now()
		if err := s.Store.UpdateExtension(ctx, e); err != nil {
			return err
		}
		typ := EventExtensionDisabled
		if enabled {
			typ = EventExtensionEnabled
		}
		seq, err = s.emit(ctx, by, typ, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug, Reason: e.DisabledReason})
		return err
	})
	if err != nil {
		return Extension{}, err
	}
	s.notify(seq)
	return e, nil
}

// SetExtensionApprovalOff switches off (or on) the approval of the
// extension's write and external tools. Only the owner.
func (s *Service) SetExtensionApprovalOff(ctx context.Context, by identity.Actor, id ids.ID, off bool) (Extension, error) {
	if by.Kind != identity.KindHuman || by.Role != identity.RoleOwner || by.Halted {
		return Extension{}, ErrForbidden
	}
	var e Extension
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if e, err = s.extension(ctx, by, id); err != nil {
			return err
		}
		if e.ApprovalOff == off {
			return nil
		}
		e.ApprovalOff, e.UpdatedAt = off, s.Clock.Now()
		if err := s.Store.UpdateExtension(ctx, e); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventApprovalOffChanged, "extension", e.ID, ConnectorChanged{ConnectorID: e.ID.String(), Slug: e.Slug, ApprovalOff: &off})
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return e, err
}

// TakeOver makes the caller the extension's maintainer (when the former
// one lost the right or left: the extension is refused until then).
func (s *Service) TakeOver(ctx context.Context, by identity.Actor, id ids.ID) (Extension, error) {
	if err := admin(by); err != nil {
		return Extension{}, err
	}
	var e Extension
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if e, err = s.extension(ctx, by, id); err != nil {
			return err
		}
		e.MaintainerID, e.UpdatedAt = by.ID, s.Clock.Now()
		if err := s.Store.UpdateExtension(ctx, e); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventTakenOver, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug})
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return e, err
}

// DeleteExtension removes an extension: grants revoked, its bot archived.
// Its versions, approvals and calls stay in the log.
func (s *Service) DeleteExtension(ctx context.Context, by identity.Actor, id ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		e, err := s.extension(ctx, by, id)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		if err := s.Store.RevokeGrantsForTarget(ctx, by.WorkspaceID, TargetExtension, e.ID, now); err != nil {
			return err
		}
		if _, err := s.Bots.ArchiveAgent(ctx, by, e.BotID); err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
		if err := s.Store.DeleteExtension(ctx, by.WorkspaceID, e.ID, now); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventExtensionDeleted, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug})
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return err
}

// ExtensionView is an extension with its versions (without code) and
// which of them were approved.
type ExtensionView struct {
	Extension
	BotHandle      string
	MaintainerName string
	Versions       []Version
	Approved       map[int]bool
}

// ListExtensions returns the workspace's extensions (owner and admins).
func (s *Service) ListExtensions(ctx context.Context, by identity.Actor) ([]ExtensionView, error) {
	if err := admin(by); err != nil {
		return nil, err
	}
	es, err := s.Store.ListExtensions(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ExtensionView, 0, len(es))
	for _, e := range es {
		v, err := s.extView(ctx, e)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// GetExtension returns one extension (owner and admins).
func (s *Service) GetExtension(ctx context.Context, by identity.Actor, id ids.ID) (ExtensionView, error) {
	if err := admin(by); err != nil {
		return ExtensionView{}, err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return ExtensionView{}, err
	}
	return s.extView(ctx, e)
}

func (s *Service) extView(ctx context.Context, e Extension) (ExtensionView, error) {
	vs, err := s.Store.ListVersions(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return ExtensionView{}, err
	}
	as, err := s.Store.ListApprovals(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return ExtensionView{}, err
	}
	v := ExtensionView{Extension: e, Versions: vs, Approved: map[int]bool{}}
	for _, a := range as {
		v.Approved[a.Version] = true
	}
	if b, err := s.Directory.ActorByID(ctx, e.BotID); err == nil {
		v.BotHandle = b.Handle
	}
	if m, err := s.Directory.ActorByID(ctx, e.MaintainerID); err == nil {
		v.MaintainerName = m.Handle
	}
	return v, nil
}

// GetVersion returns a version with its source (owner and admins).
func (s *Service) GetVersion(ctx context.Context, by identity.Actor, id ids.ID, version int) (Version, Diff, error) {
	if err := admin(by); err != nil {
		return Version{}, Diff{}, err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return Version{}, Diff{}, err
	}
	v, err := s.Store.GetVersion(ctx, by.WorkspaceID, e.ID, version)
	if err != nil {
		return Version{}, Diff{}, ErrVersionNotFound
	}
	live, err := s.liveVersion(ctx, e)
	if err != nil {
		return Version{}, Diff{}, err
	}
	return v, DiffVersions(live, v), nil
}

// TestTool runs a tool of any version (not yet approved too) as the
// extension's bot, for the person who builds it. It is a real call: a
// person started it, so it asks no approval; it is logged as a test.
func (s *Service) TestTool(ctx context.Context, by identity.Actor, id ids.ID, version int, tool string, input json.RawMessage) (Result, []string, error) {
	if err := admin(by); err != nil {
		return Result{}, nil, err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return Result{}, nil, err
	}
	v, err := s.Store.GetVersion(ctx, by.WorkspaceID, e.ID, version)
	if err != nil {
		return Result{}, nil, ErrVersionNotFound
	}
	mt, ok := v.Manifest.Tool(tool)
	if !ok {
		return Result{}, nil, ErrUnknownTool
	}
	bot, err := s.Directory.ActorByID(ctx, e.BotID)
	if err != nil {
		return Result{}, nil, err
	}
	start := s.Clock.Now()
	res, logs, err := s.runTool(ctx, runCtx{ext: e, version: v, tool: mt, caller: bot, input: input})
	call := ToolCall{ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, ActorID: by.ID, TargetKind: TargetExtension, TargetID: e.ID,
		Tool: fmt.Sprintf("%s (test v%d)", tool, version), Risk: mt.Risk, StartedAt: start, InBytes: len(input)}
	s.finishCall(ctx, &call, start, res, err)
	return res, logs, err
}

// runCtx is one extension tool call in progress.
type runCtx struct {
	ext     Extension
	version Version
	tool    ManifestTool
	caller  identity.Actor
	input   json.RawMessage
	depth   int
}

// runTool executes a tool: an http tool here, a js tool in the runner.
func (s *Service) runTool(ctx context.Context, rc runCtx) (Result, []string, error) {
	switch rc.tool.Kind {
	case ToolHTTP:
		res, err := s.runHTTPTool(ctx, rc)
		return res, nil, err
	case ToolJS:
		if s.Engine == nil {
			return Result{}, nil, extrunner.ErrNoRunner
		}
		lim := s.Limits.Run
		out, err := s.Engine.Run(ctx, extrunner.Job{Bundle: rc.version.Bundle, Fn: rc.tool.Fn, Input: rc.input, Limits: lim},
			func(ctx context.Context, kind string, payload json.RawMessage) (json.RawMessage, error) {
				return s.host(ctx, rc, kind, payload)
			})
		if err != nil {
			return Result{}, nil, err
		}
		text := string(out.Output)
		var str string
		if json.Unmarshal(out.Output, &str) == nil {
			text = str
		}
		return Result{Text: text}, out.Logs, nil
	}
	return Result{}, nil, ErrUnknownTool
}

// checkMaintainer refuses an extension whose maintainer is no longer an
// owner or admin: its rights came from that person (docs/25 5.).
func (s *Service) checkMaintainer(ctx context.Context, e Extension) error {
	m, err := s.Directory.ActorByID(ctx, e.MaintainerID)
	if err != nil || m.ArchivedAt != nil || m.Halted || m.State != identity.StateActive || !m.IsAdmin() || m.Kind != identity.KindHuman {
		return ErrMaintainerGone
	}
	return nil
}

// recordOutcome counts failures for the automatic switch-off.
func (s *Service) recordOutcome(ctx context.Context, e Extension, failed bool) {
	if !failed {
		if e.ConsecutiveErrors != 0 {
			_ = s.Store.SetExtensionErrors(ctx, e.WorkspaceID, e.ID, 0, e.RecentErrors)
		}
		return
	}
	now := s.Clock.Now()
	lim := s.limits()
	recent := []time.Time{now}
	for _, t := range e.RecentErrors {
		if now.Sub(t) < lim.DisableWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) > lim.DisableErrors {
		recent = recent[:lim.DisableErrors]
	}
	consecutive := e.ConsecutiveErrors + 1
	_ = s.Store.SetExtensionErrors(ctx, e.WorkspaceID, e.ID, consecutive, recent)
	if consecutive < lim.DisableConsecutive && len(recent) < lim.DisableErrors {
		return
	}
	reason := fmt.Sprintf("automatic: %d failed calls in a row", consecutive)
	if consecutive < lim.DisableConsecutive {
		reason = fmt.Sprintf("automatic: %d failed calls in %s", len(recent), lim.DisableWindow)
	}
	e.State, e.DisabledReason, e.UpdatedAt = StateDisabled, reason, now
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.UpdateExtension(ctx, e); err != nil {
			return err
		}
		ev := ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug, Reason: reason}
		seq, err := s.emit(ctx, identity.Actor{ID: e.BotID, WorkspaceID: e.WorkspaceID}, EventExtensionDisabled, "extension", e.ID, ev)
		s.notify(seq)
		return err
	})
	if err != nil && s.Logger != nil {
		s.Logger.Warn("workshop: automatic switch-off", "extension", e.Slug, "err", err)
	}
}

func (s *Service) limits() Limits {
	l := s.Limits
	d := DefaultLimits
	if l.Run.Timeout <= 0 {
		l.Run = d.Run
	}
	if l.ConcurrencyWorkspace <= 0 {
		l.ConcurrencyWorkspace = d.ConcurrencyWorkspace
	}
	if l.DisableConsecutive <= 0 {
		l.DisableConsecutive = d.DisableConsecutive
	}
	if l.DisableErrors <= 0 {
		l.DisableErrors = d.DisableErrors
	}
	if l.DisableWindow <= 0 {
		l.DisableWindow = d.DisableWindow
	}
	return l
}

// finishCall logs a call's outcome.
func (s *Service) finishCall(ctx context.Context, call *ToolCall, start time.Time, res Result, err error) {
	call.Outcome = OutcomeOK
	switch {
	case errors.Is(err, extrunner.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		call.Outcome = OutcomeTimeout
	case errors.Is(err, extrunner.ErrMemory), errors.Is(err, extrunner.ErrOutput), errors.Is(err, extrunner.ErrHostCalls),
		errors.Is(err, ErrBudget), errors.Is(err, ErrBusy), errors.Is(err, extrunner.ErrBusy):
		call.Outcome = OutcomeLimit
	case err != nil, res.IsError:
		call.Outcome = OutcomeError
	}
	if err != nil {
		call.Error = errText(err)
	}
	call.OutBytes = len(res.Text)
	call.DurationMS = int(s.Clock.Now().Sub(start) / time.Millisecond)
	if call.DurationMS < 0 {
		call.DurationMS = 0
	}
	if lerr := s.Store.InsertToolCall(ctx, *call); lerr != nil && s.Logger != nil {
		s.Logger.Warn("workshop: call log", "err", lerr)
	}
}

// acquire takes one of the workspace's concurrent call slots.
func (s *Service) acquire(workspaceID ids.ID) (func(), error) {
	s.slotsMu.Lock()
	if s.slots == nil {
		s.slots = map[ids.ID]chan struct{}{}
	}
	ch, ok := s.slots[workspaceID]
	if !ok {
		ch = make(chan struct{}, s.limits().ConcurrencyWorkspace)
		s.slots[workspaceID] = ch
	}
	s.slotsMu.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	default:
		return nil, ErrBusy
	}
}

// checkBudget refuses a call when the workspace's plan budget is used up.
func (s *Service) checkBudget(ctx context.Context, workspaceID ids.ID) error {
	if s.Budget == nil {
		return nil
	}
	b, err := s.Budget(ctx, workspaceID)
	if err != nil || b == nil {
		return err
	}
	now := s.Clock.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	n, err := s.Store.CountCallsSince(ctx, workspaceID, month)
	if err != nil {
		return err
	}
	if n >= *b {
		return ErrBudget
	}
	return nil
}
