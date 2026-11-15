// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/dbstats"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// testFileStorage: round 54. Deleted files, "My files", the workspace usage
// (each content once per workspace), the content sweep across workspaces
// and the database share.
func testFileStorage(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	other := identity.Workspace{ID: gen.New(), Name: "other", CreatedAt: t0, Slug: "other"}
	octx := db.WithWorkspace(context.Background(), other.ID)
	must(t, b.Identity.CreateWorkspace(octx, other))
	oHuman := identity.Actor{ID: gen.New(), WorkspaceID: other.ID, Kind: identity.KindHuman, Handle: "olga",
		DisplayName: "Olga", Role: identity.RoleOwner, State: "active", CreatedAt: t0}
	must(t, b.Identity.CreateActor(octx, oHuman))
	oChannel := conversation.Channel{ID: gen.New(), WorkspaceID: other.ID, Kind: conversation.KindPublic,
		Name: "general", CreatedBy: oHuman.ID, CreatedAt: t0}
	must(t, b.Conversation.CreateChannel(octx, oChannel))
	sys := b.System(context.Background())

	const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	file := func(ws, by, ch ids.ID, sha, mime string, size int64, m int) files.File {
		return files.File{ID: gen.New(), WorkspaceID: ws, UploadedBy: by, ChannelID: ch, Name: "f.bin", Mime: mime,
			Size: size, SHA256: sha, CreatedAt: t0.Add(time.Duration(m) * time.Minute)}
	}
	f1 := file(f.ws.ID, f.human.ID, f.channel.ID, shaA, "application/pdf", 100, 1)
	f2 := file(f.ws.ID, f.human.ID, f.channel.ID, shaA, "image/png", 100, 2) // the same content again
	f3 := file(f.ws.ID, f.agent.ID, f.channel.ID, shaB, "application/zip", 50, 3)
	f4 := file(other.ID, oHuman.ID, oChannel.ID, shaA, "application/pdf", 100, 4) // A in another workspace
	// Someone else in the same workspace holds A too: it must not show in
	// the caller's shared count (that would reveal who holds which file).
	peer := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindHuman, Handle: "peer",
		DisplayName: "Peer", Role: identity.RoleMember, State: "active", CreatedAt: t0}
	must(t, b.Identity.CreateActor(ctx, peer))
	f5 := file(f.ws.ID, peer.ID, f.channel.ID, shaA, "application/pdf", 100, 5)
	for _, x := range []files.File{f1, f2, f3, f5} {
		must(t, b.Files.Insert(ctx, x))
	}
	must(t, b.Files.Insert(octx, f4))
	msg := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: f.human.ID,
		Kind: "text", Body: "files", CreatedAt: t0}
	must(t, b.Conversation.InsertMessage(ctx, msg))
	must(t, b.Files.AttachToMessage(ctx, msg.ID, f1.ID, 0))

	// The usage counts each content once per workspace.
	if n, err := b.Files.LiveFilesBytes(ctx, f.ws.ID); err != nil || n != 150 {
		t.Fatalf("live bytes = %d, %v; want 150 (A once, B)", n, err)
	}
	if has, err := b.Files.WorkspaceHasLiveContent(ctx, f.ws.ID, shaA); err != nil || !has {
		t.Fatalf("workspace has A: %v %v", has, err)
	}
	if has, err := b.Files.WorkspaceHasLiveContent(ctx, f.ws.ID, "cccc"); err != nil || has {
		t.Fatalf("workspace has an unknown content: %v %v", has, err)
	}

	// "My files": newest first, the message, the shared count, filters, paging.
	mine, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{f.human.ID}, Limit: 10})
	must(t, err)
	if len(mine) != 2 || mine[0].ID != f2.ID || mine[1].ID != f1.ID {
		t.Fatalf("my files = %+v, want f2, f1", mine)
	}
	if mine[1].MessageID != msg.ID || mine[0].MessageID != ids.Nil {
		t.Fatalf("message links: %v %v", mine[1].MessageID, mine[0].MessageID)
	}
	if mine[0].SharedCount != 1 || mine[1].SharedCount != 1 {
		t.Fatalf("shared counts %d %d, want 1 1 (another person's and the other workspace's copies do not count)", mine[0].SharedCount, mine[1].SharedCount)
	}
	// And the other way round: the peer learns nothing of the caller's copies.
	if got, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{peer.ID}, Limit: 10}); err != nil || len(got) != 1 || got[0].SharedCount != 0 {
		t.Fatalf("peer's files: %+v %v; want f5 with shared count 0", got, err)
	}
	// Out of the way for the usage and sweep checks below (A stays removable).
	must(t, b.Files.MarkDeleted(ctx, f5.ID, t0.Add(time.Minute)))
	for kind, want := range map[string]ids.ID{files.KindImage: f2.ID, files.KindDocument: f1.ID} {
		got, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{f.human.ID}, Kind: kind, Limit: 10})
		if err != nil || len(got) != 1 || got[0].ID != want {
			t.Fatalf("kind %s: %+v %v", kind, got, err)
		}
	}
	if got, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{f.human.ID}, Kind: files.KindOther, Limit: 10}); err != nil || len(got) != 0 {
		t.Fatalf("kind other: %+v %v", got, err)
	}
	page, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{f.human.ID},
		BeforeAt: &f2.CreatedAt, BeforeID: f2.ID, Limit: 10})
	if err != nil || len(page) != 1 || page[0].ID != f1.ID {
		t.Fatalf("second page: %+v %v", page, err)
	}
	if got, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{f.agent.ID}, ChannelID: f.channel.ID, Limit: 1}); err != nil || len(got) != 1 || got[0].ID != f3.ID {
		t.Fatalf("agent's files: %+v %v", got, err)
	}

	// Deleting: soft, once; the attachment keeps only its ID.
	at := t0.Add(time.Hour)
	must(t, b.Files.MarkDeleted(ctx, f1.ID, at))
	if err := b.Files.MarkDeleted(ctx, f1.ID, at); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("second delete: %v, want ErrConflict", err)
	}
	if got, err := b.Files.ByID(ctx, f1.ID); err != nil || got.DeletedAt == nil || !got.DeletedAt.Equal(at) {
		t.Fatalf("deleted row: %+v %v", got, err)
	}
	atts, err := b.Files.AttachmentsForMessage(ctx, msg.ID)
	if err != nil || len(atts) != 1 || !atts[0].Deleted || atts[0].Name != "" || atts[0].Size != 0 {
		t.Fatalf("deleted attachment: %+v %v", atts, err)
	}
	batch, err := b.Files.AttachmentsForMessages(ctx, []ids.ID{msg.ID})
	if err != nil || len(batch[msg.ID]) != 1 || !batch[msg.ID][0].Deleted {
		t.Fatalf("deleted attachment in batch: %+v %v", batch, err)
	}
	if list, err := b.Files.ListByChannel(ctx, f.channel.ID); err != nil || slices.ContainsFunc(list, func(x files.File) bool { return x.ID == f1.ID }) {
		t.Fatalf("channel list shows the deleted file: %v", err)
	}
	if mine, err := b.Files.ListMine(ctx, files.MyFilesQuery{WorkspaceID: f.ws.ID, UploaderIDs: []ids.ID{f.human.ID}, Limit: 10}); err != nil || len(mine) != 1 || mine[0].SharedCount != 0 {
		t.Fatalf("my files after delete: %+v %v", mine, err)
	}
	// f2 still holds A in this workspace.
	if n, _ := b.Files.LiveFilesBytes(ctx, f.ws.ID); n != 150 {
		t.Fatalf("live bytes after deleting one copy = %d, want 150", n)
	}
	must(t, b.Files.MarkDeleted(ctx, f2.ID, at))
	if n, _ := b.Files.LiveFilesBytes(ctx, f.ws.ID); n != 50 {
		t.Fatalf("live bytes after deleting both copies = %d, want 50", n)
	}

	// The sweep looks across workspaces: the other workspace still uses A.
	hashes, err := b.Files.RemovableContents(sys, 10)
	if err != nil || !slices.Equal(hashes, []string{shaA}) {
		t.Fatalf("removable = %v, %v; want [A]", hashes, err)
	}
	if live, err := b.Files.ContentLive(sys, shaA); err != nil || !live {
		t.Fatalf("A is live in the other workspace: %v %v", live, err)
	}
	must(t, b.Files.MarkDeleted(octx, f4.ID, at))
	if live, err := b.Files.ContentLive(sys, shaA); err != nil || live {
		t.Fatalf("A after every copy is deleted: %v %v", live, err)
	}
	must(t, b.Files.MarkContentRemoved(sys, shaA, at))
	if hashes, err := b.Files.RemovableContents(sys, 10); err != nil || len(hashes) != 0 {
		t.Fatalf("removable after the sweep = %v, %v", hashes, err)
	}
	must(t, b.Files.LockContent(ctx, shaA)) // outside a transaction it is harmless

	// The usage row.
	if u, err := b.Files.UsageOf(ctx, f.ws.ID); err != nil || u.QuotaBytes != 0 || u.WorkspaceID != f.ws.ID {
		t.Fatalf("missing usage row: %+v %v", u, err)
	}
	must(t, b.Tx.InTx(ctx, func(ctx context.Context) error {
		u, err := b.Files.LockUsage(ctx, f.ws.ID, t0)
		if err != nil {
			return err
		}
		measured := t0.Add(time.Minute)
		u.QuotaBytes, u.FilesBytes, u.DBBytes, u.ChatBytes, u.DBMeasuredAt, u.WarnedLevel, u.UpdatedAt =
			1000, 50, 800, 300, &measured, files.Level80, t0.Add(2*time.Minute)
		return b.Files.SaveUsage(ctx, u)
	}))
	u, err := b.Files.UsageOf(ctx, f.ws.ID)
	if err != nil || u.QuotaBytes != 1000 || u.FilesBytes != 50 || u.DBBytes != 800 || u.ChatBytes != 300 ||
		u.WarnedLevel != files.Level80 || u.DBMeasuredAt == nil || u.State() != files.StorageWarning80 {
		t.Fatalf("saved usage: %+v %v", u, err)
	}
	// Round 51b: the plan's copy in the storage row, and the effective quota.
	must(t, b.Tx.InTx(ctx, func(ctx context.Context) error {
		u, err := b.Files.LockUsage(ctx, f.ws.ID, t0)
		if err != nil {
			return err
		}
		u.Plan = &files.Plan{ID: gen.New(), Name: "Közepes", StorageBytes: 700, CPUCores: 4, MemoryBytes: 4 << 30}
		return b.Files.SaveUsage(ctx, u)
	}))
	if u, err := b.Files.UsageOf(ctx, f.ws.ID); err != nil || u.Plan == nil || u.Plan.Name != "Közepes" || u.Plan.StorageBytes != 700 ||
		u.Plan.CPUCores != 4 || u.Plan.MemoryBytes != 4<<30 || u.QuotaBytes != 1000 || u.Limit() != 700 {
		t.Fatalf("usage with a plan: %+v %v", u, err)
	}
	must(t, b.Tx.InTx(ctx, func(ctx context.Context) error {
		u, err := b.Files.LockUsage(ctx, f.ws.ID, t0)
		if err != nil {
			return err
		}
		u.Plan = nil
		return b.Files.SaveUsage(ctx, u)
	}))
	if u, err := b.Files.UsageOf(ctx, f.ws.ID); err != nil || u.Plan != nil || u.Limit() != 1000 {
		t.Fatalf("usage after the plan is removed: %+v %v", u, err)
	}
	if u, err := b.Files.UsageOf(octx, other.ID); err != nil || u.QuotaBytes != 0 {
		t.Fatalf("the other workspace's usage: %+v %v", u, err)
	}

	// The database share (Postgres only).
	list, err := b.DBStats.WorkspaceIDs(sys)
	if errors.Is(err, dbstats.ErrNoDatabase) {
		return
	}
	must(t, err)
	if !slices.Contains(list, f.ws.ID) || !slices.Contains(list, other.ID) {
		t.Fatalf("workspaces = %v", list)
	}
	rb, err := b.DBStats.WorkspaceRowBytes(ctx, f.ws.ID)
	if err != nil || rb.Chat <= 0 || rb.Other <= 0 {
		t.Fatalf("row bytes = %+v, %v; want messages and files counted", rb, err)
	}
	// Another workspace's rows are not counted (and row-level security
	// would hide them anyway).
	if orb, err := b.DBStats.WorkspaceRowBytes(octx, other.ID); err != nil || orb.Chat != 0 || orb.Other <= 0 {
		t.Fatalf("other row bytes = %+v, %v", orb, err)
	}
	if n, err := b.DBStats.ChatTablesSize(ctx); err != nil || n <= 0 {
		t.Fatalf("chat tables = %d, %v", n, err)
	}
}
