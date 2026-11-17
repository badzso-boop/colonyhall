// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/platform/ids"
)

// The Builder (round 56d, docs/25 13.10) is an ordinary agent with the
// "workshop" grant whose owner is an owner or an admin. Its meta-tools act
// for that person, never more: it drafts and tests versions, creates
// channels; making a version live needs a person's approval on a card in
// its conversation, and the person who allows it is the one who approves.

// Errors of the Builder.
var (
	ErrNotBuilder     = errors.New("this agent is not a Builder: it needs the Workshop grant and an owner who is an owner or an admin")
	ErrNotApprovedNow = errors.New("the person did not approve it")
)

// Wiki writes the extensions' build logs (the wiki module).
type Wiki interface {
	Get(ctx context.Context, by identity.Actor, slug string) (wiki.Page, error)
	Create(ctx context.Context, by identity.Actor, in wiki.CreateInput) (wiki.Page, error)
	Edit(ctx context.Context, by identity.Actor, slug string, in wiki.EditInput) (wiki.Page, error)
}

// BuilderOwner returns the person a Builder agent acts for.
func (s *Service) BuilderOwner(ctx context.Context, agent identity.Actor) (identity.Actor, error) {
	if agent.Kind != identity.KindAgent || agent.Halted || agent.OwnerID == ids.Nil || !s.granted(ctx, agent, TargetWorkshop, ids.Nil) {
		return identity.Actor{}, ErrNotBuilder
	}
	owner, err := s.Directory.ActorByID(ctx, agent.OwnerID)
	if err != nil || owner.Kind != identity.KindHuman || !owner.IsAdmin() || owner.Halted || owner.ArchivedAt != nil ||
		owner.WorkspaceID != agent.WorkspaceID {
		return identity.Actor{}, ErrNotBuilder
	}
	return owner, nil
}

// IsBuilder reports whether the agent gets the meta-tools.
func (s *Service) IsBuilder(ctx context.Context, agent identity.Actor) bool {
	_, err := s.BuilderOwner(ctx, agent)
	return err == nil
}

// Overview is what a Builder sees of the Workshop.
type Overview struct {
	Connectors []ConnectorView
	Extensions []ExtensionView
}

// BuilderOverview lists the connectors and extensions for a Builder.
func (s *Service) BuilderOverview(ctx context.Context, agent identity.Actor) (Overview, error) {
	owner, err := s.BuilderOwner(ctx, agent)
	if err != nil {
		return Overview{}, err
	}
	cs, err := s.ListConnectors(ctx, owner)
	if err != nil {
		return Overview{}, err
	}
	es, err := s.ListExtensions(ctx, owner)
	return Overview{Connectors: cs, Extensions: es}, err
}

// BuilderExtensionID finds an extension by its short name for a Builder.
func (s *Service) BuilderExtensionID(ctx context.Context, agent identity.Actor, slug string) (identity.Actor, ids.ID, error) {
	owner, err := s.BuilderOwner(ctx, agent)
	if err != nil {
		return identity.Actor{}, ids.Nil, err
	}
	e, err := s.extensionBySlug(ctx, agent.WorkspaceID, strings.TrimPrefix(strings.TrimPrefix(slug, GatewayPrefix), "@"))
	if err != nil {
		return owner, ids.Nil, ErrNotFound
	}
	return owner, e.ID, nil
}

// BuilderDraft stores a new version written by a Builder (its author is
// the agent, for the trace).
func (s *Service) BuilderDraft(ctx context.Context, agent identity.Actor, id ids.ID, in VersionInput) (Version, Diff, error) {
	owner, err := s.BuilderOwner(ctx, agent)
	if err != nil {
		return Version{}, Diff{}, err
	}
	in.Via = agent.ID
	return s.AddVersion(ctx, owner, id, in)
}

// BuilderPublish asks a person, on an approval card in the Builder's
// conversation, to make a version live (or, with rollback, an earlier
// one live again), and does it when they allow it, as them.
func (s *Service) BuilderPublish(ctx context.Context, agent identity.Actor, id ids.ID, version int, rollback bool,
	channelID, threadID ids.ID, current func(context.Context) (identity.Actor, error)) (Extension, Diff, error) {
	owner, err := s.BuilderOwner(ctx, agent)
	if err != nil {
		return Extension{}, Diff{}, err
	}
	if channelID == ids.Nil {
		return Extension{}, Diff{}, ErrNoChannel
	}
	e, err := s.extension(ctx, owner, id)
	if err != nil {
		return Extension{}, Diff{}, err
	}
	_, diff, err := s.GetVersion(ctx, owner, id, version)
	if err != nil {
		return Extension{}, Diff{}, err
	}
	what := "publish"
	if rollback {
		what = "roll back to"
	}
	summary, _ := json.Marshal(map[string]any{"extension": e.Slug, "version": version, "adds": diff})
	req, err := s.Approvals.CreateRequest(ctx, agent, approval.CreateInput{ChannelID: channelID, ThreadRootID: threadID,
		ToolName: fmt.Sprintf("Workshop: %s %s v%d", what, e.Slug, version), Input: string(summary)})
	if err != nil {
		return Extension{}, Diff{}, err
	}
	decided, err := s.Approvals.WaitForDecision(ctx, req, current)
	if err != nil {
		return Extension{}, Diff{}, err
	}
	if decided.State != approval.StateAllowed {
		return Extension{}, diff, ErrNotApprovedNow
	}
	decider, err := s.Directory.ActorByID(ctx, decided.DecidedBy)
	if err != nil || decider.Kind != identity.KindHuman || !decider.IsAdmin() {
		return Extension{}, diff, ErrForbidden
	}
	if rollback {
		e, err = s.Rollback(ctx, decider, id, version)
		return e, diff, err
	}
	e, _, err = s.ApproveVersion(ctx, decider, id, version)
	return e, diff, err
}

// BuilderCreateChannel creates a channel for a Builder's routines, as its
// owner (agents may not create channels themselves), with the Builder in it.
func (s *Service) BuilderCreateChannel(ctx context.Context, agent identity.Actor, name, description string, private bool) (ids.ID, error) {
	owner, err := s.BuilderOwner(ctx, agent)
	if err != nil {
		return ids.Nil, err
	}
	kind := conversation.KindPublic
	if private {
		kind = conversation.KindPrivate
	}
	c, err := s.Channels.CreateChannel(ctx, owner, name, description, kind)
	if err != nil {
		return ids.Nil, err
	}
	if !s.Channels.IsMember(ctx, c.ID, agent.ID) {
		if err := s.Channels.AddMember(ctx, owner, c.ID, agent.ID, true); err != nil {
			return c.ID, err
		}
	}
	return c.ID, nil
}

// --- the build log (docs/25 4.)

// BuildLogTitle is the title of an extension's wiki page.
func BuildLogTitle(slug string) string { return "Műhely – " + slug }

// writeBuildLog updates the extension's page in the wiki (best effort:
// the extension works without it). The maintainer writes it.
func (s *Service) writeBuildLog(ctx context.Context, e Extension) {
	if s.Wiki == nil {
		return
	}
	maintainer, err := s.Directory.ActorByID(ctx, e.MaintainerID)
	if err != nil || maintainer.Kind != identity.KindHuman {
		return
	}
	body, err := s.buildLog(ctx, e)
	if err != nil {
		return
	}
	title := BuildLogTitle(e.Slug)
	slug := wiki.Slugify(title)
	page, err := s.Wiki.Get(ctx, maintainer, slug)
	if err == nil && page.DeletedAt == nil {
		_, err = s.Wiki.Edit(ctx, maintainer, slug, wiki.EditInput{BaseRev: page.Rev, Body: body, Title: title})
	} else {
		_, err = s.Wiki.Create(ctx, maintainer, wiki.CreateInput{Title: title, Body: body, Tags: []string{"muhely"}})
	}
	if err != nil && s.Logger != nil {
		s.Logger.Info("workshop: build log", "extension", e.Slug, "err", err)
	}
}

func (s *Service) handle(ctx context.Context, id ids.ID) string {
	if a, err := s.Directory.ActorByID(ctx, id); err == nil {
		return "@" + a.Handle
	}
	return "?"
}

// buildLog renders the build log: what it is, who keeps it, its versions
// with their authors and approvals, its tools and routines.
func (s *Service) buildLog(ctx context.Context, e Extension) (string, error) {
	vs, err := s.Store.ListVersions(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return "", err
	}
	as, err := s.Store.ListApprovals(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s (`%s`)\n\n", e.Title, e.Slug)
	if e.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", e.Description)
	}
	fmt.Fprintf(&b, "- **Bot:** %s\n- **Karbantartó:** %s\n- **Állapot:** %s", s.handle(ctx, e.BotID), s.handle(ctx, e.MaintainerID), e.State)
	if e.LiveVersion > 0 {
		fmt.Fprintf(&b, ", élő verzió: v%d", e.LiveVersion)
	}
	b.WriteString("\n\n")
	for _, v := range vs {
		if v.Version != e.LiveVersion {
			continue
		}
		b.WriteString("## Eszközök\n\n")
		for _, t := range v.Manifest.Tools {
			fmt.Fprintf(&b, "- `%s` (%s, %s): %s\n", ExtensionGatewayName(e.Slug, t.Name), t.Kind, t.Risk, t.Description)
		}
		if len(v.Manifest.Routines) > 0 {
			b.WriteString("\n## Rutinok\n\n")
			for _, r := range v.Manifest.Routines {
				target := r.Tool
				if r.Agent != "" {
					target = "@" + r.Agent
				}
				fmt.Fprintf(&b, "- `%s`: %s → %s", r.Name, r.Trigger.Kind(), target)
				if r.Output != "" {
					fmt.Fprintf(&b, " (#%s)", r.Output)
				}
				b.WriteString("\n")
			}
		}
		if len(v.Manifest.Connectors)+len(v.Manifest.Egress)+len(v.Manifest.Store) > 0 {
			fmt.Fprintf(&b, "\n**Kapcsolatok:** %s · **Kifelé:** %s · **Adattár:** %s\n",
				listOr(v.Manifest.Connectors), listOr(v.Manifest.Egress), listOr(v.Manifest.Store))
		}
		b.WriteString("\n")
	}
	approvals := map[int][]Approval{}
	for _, a := range as {
		approvals[a.Version] = append(approvals[a.Version], a)
	}
	b.WriteString("## Verziók\n\n| Verzió | Írta | Mikor | Jóváhagyta |\n| --- | --- | --- | --- |\n")
	for _, v := range vs {
		var by []string
		for _, a := range approvals[v.Version] {
			by = append(by, s.handle(ctx, a.DecidedBy)+" "+a.CreatedAt.UTC().Format("2006-01-02 15:04"))
		}
		sort.Strings(by)
		fmt.Fprintf(&b, "| v%d | %s | %s | %s |\n", v.Version, s.handle(ctx, v.CreatedBy), v.CreatedAt.UTC().Format("2006-01-02 15:04"), listOr(by))
	}
	for _, a := range as {
		d := a.Diff
		var parts []string
		for _, x := range []struct {
			name string
			list []string
		}{{"kapcsolat", d.NewConnectors}, {"kifelé", d.NewEgress}, {"adattár", d.NewStore}, {"eszköz", d.NewTools}, {"rutin", d.NewRoutines}} {
			if len(x.list) > 0 {
				parts = append(parts, x.name+": "+strings.Join(x.list, ", "))
			}
		}
		if len(d.RaisedRisks) > 0 {
			parts = append(parts, fmt.Sprintf("emelt kockázat: %v", d.RaisedRisks))
		}
		if d.CodeChanged {
			parts = append(parts, "kódváltozás")
		}
		if len(parts) > 0 {
			fmt.Fprintf(&b, "\n- v%d jóváhagyásakor új: %s", a.Version, strings.Join(parts, "; "))
		}
	}
	b.WriteString("\n\n_Ezt az oldalt a Műhely írja minden verzió és jóváhagyás után._\n")
	return b.String(), nil
}

func listOr(xs []string) string {
	if len(xs) == 0 {
		return "–"
	}
	return strings.Join(xs, ", ")
}

// --- export and import (docs/25 13.15/2: a folder a Gitea repository can hold)

// BundleFormat names the export format.
const BundleFormat = "colonyhall-extension/1"

// Bundle is an exported extension: files by path, no secrets.
type Bundle struct {
	Format      string            `json:"format"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Version     int               `json:"version"`
	ExportedAt  time.Time         `json:"exported_at"`
	Files       map[string]string `json:"files"`
}

// Export returns a version (0: the live one, else the newest) as a bundle.
func (s *Service) Export(ctx context.Context, by identity.Actor, id ids.ID, version int) (Bundle, error) {
	if err := admin(by); err != nil {
		return Bundle{}, err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return Bundle{}, err
	}
	if version == 0 {
		version = e.LiveVersion
	}
	if version == 0 {
		vs, err := s.Store.ListVersions(ctx, e.WorkspaceID, e.ID)
		if err != nil || len(vs) == 0 {
			return Bundle{}, ErrVersionNotFound
		}
		version = vs[0].Version
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, version)
	if err != nil {
		return Bundle{}, ErrVersionNotFound
	}
	manifest, _ := json.MarshalIndent(v.Manifest, "", "  ")
	files := map[string]string{"manifest.json": string(manifest) + "\n"}
	if v.Source != "" {
		files["src/extension.ts"] = v.Source
	}
	if log, err := s.buildLog(ctx, e); err == nil {
		files["BUILDLOG.md"] = log
	}
	return Bundle{Format: BundleFormat, Slug: e.Slug, Title: e.Title, Description: e.Description, Version: version,
		ExportedAt: s.Clock.Now(), Files: files}, nil
}

// ImportInput imports a bundle as a new extension (a draft).
type ImportInput struct {
	Bundle    Bundle
	Slug      string // empty: the bundle's
	BotHandle string
}

// Import creates an extension from a bundle; its version 1 is a draft
// that a person approves as any other (the connectors it names must exist).
func (s *Service) Import(ctx context.Context, by identity.Actor, in ImportInput) (Extension, Version, Diff, error) {
	if err := admin(by); err != nil {
		return Extension{}, Version{}, Diff{}, err
	}
	b := in.Bundle
	if b.Format != BundleFormat {
		return Extension{}, Version{}, Diff{}, fmt.Errorf("%w: unknown bundle format %q", ErrBadManifest, b.Format)
	}
	var m Manifest
	if err := json.Unmarshal([]byte(b.Files["manifest.json"]), &m); err != nil {
		return Extension{}, Version{}, Diff{}, fmt.Errorf("%w: manifest.json: %s", ErrBadManifest, err.Error())
	}
	slug := in.Slug
	if slug == "" {
		slug = b.Slug
	}
	e, err := s.CreateExtension(ctx, by, ExtensionInput{Slug: slug, Title: b.Title, Description: b.Description, BotHandle: in.BotHandle})
	if err != nil {
		return Extension{}, Version{}, Diff{}, err
	}
	v, d, err := s.AddVersion(ctx, by, e.ID, VersionInput{Manifest: m, Source: b.Files["src/extension.ts"]})
	if err != nil {
		// The empty extension stays a draft the person can fix or delete.
		return e, Version{}, Diff{}, err
	}
	return e, v, d, nil
}
