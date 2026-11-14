// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/files/memory"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// convStub lets everyone write to every channel.
type convStub struct{}

func (convStub) CanRead(context.Context, identity.Actor, ids.ID) (bool, error)  { return true, nil }
func (convStub) CanWrite(context.Context, identity.Actor, ids.ID) (bool, error) { return true, nil }
func (convStub) MessageByID(context.Context, ids.ID) (conversation.Message, error) {
	return conversation.Message{}, db.ErrNotFound
}
func (convStub) MemberIDs(context.Context, ids.ID) ([]ids.ID, error) { return nil, nil }
func (convStub) ChannelByID(_ context.Context, id ids.ID) (conversation.Channel, error) {
	return conversation.Channel{ID: id, Kind: conversation.KindPublic, Name: "general"}, nil
}

// hookedStorage runs onDelete while the sweep removes a content (inside its
// transaction, after it found no live row).
type hookedStorage struct {
	*memory.Storage
	onDelete func()
}

// Put skips a content that is already there, like the filesystem store.
func (h *hookedStorage) Put(ctx context.Context, sha string, r io.Reader) (int64, error) {
	if n, err := h.Stat(ctx, sha); err == nil {
		return n, nil
	}
	return h.Storage.Put(ctx, sha, r)
}

func (h *hookedStorage) Delete(ctx context.Context, sha string) error {
	if h.onDelete != nil {
		h.onDelete()
	}
	return h.Storage.Delete(ctx, sha)
}

type warnings struct {
	mu   sync.Mutex
	seen []files.StorageState
}

func (w *warnings) StorageWarning(_ context.Context, _ ids.ID, u files.Usage) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seen = append(w.seen, u.State())
}

func newFilesService(t *testing.T) (*files.Service, *hookedStorage, *warnings, identity.Actor) {
	t.Helper()
	st := &hookedStorage{Storage: memory.NewStorage()}
	w := &warnings{}
	svc := &files.Service{Store: memory.NewStore(), Storage: st, Conversation: convStub{}, Tx: &db.Memory{},
		Clock: clock.NewManual(time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)), IDs: &ids.Sequential{}, Notifier: w}
	by := identity.Actor{ID: ids.V7{}.New(), WorkspaceID: ids.V7{}.New(), Kind: identity.KindHuman, Role: identity.RoleOwner}
	return svc, st, w, by
}

// A sweep that removes a content while the same content is being uploaded
// again: the upload waits for the sweep's lock and writes the content anew,
// so the new file downloads. Were the content written outside the lock, the
// upload would find it still there, skip the write, and lose it to the
// sweep.
func TestSweepAndAConcurrentUploadOfTheSameContent(t *testing.T) {
	svc, st, _, by := newFilesService(t)
	ctx := context.Background()
	ch := ids.V7{}.New()
	content := []byte("the same bytes")
	old, err := svc.Upload(ctx, by, ch, "a.txt", bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, by, old.ID); err != nil {
		t.Fatal(err)
	}
	var again files.File
	var upErr error
	done := make(chan struct{})
	st.onDelete = func() {
		st.onDelete = nil
		go func() {
			defer close(done)
			again, upErr = svc.Upload(ctx, by, ch, "b.txt", bytes.NewReader(content))
		}()
		time.Sleep(50 * time.Millisecond) // let the upload reach the lock
	}
	n, err := svc.SweepContents(ctx)
	if err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	<-done
	if upErr != nil {
		t.Fatal(upErr)
	}
	_, rc, err := svc.OpenContent(ctx, by, again.ID)
	if err != nil {
		t.Fatalf("the new upload's content is gone: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("content = %q", got)
	}
	// The next sweep keeps it: a live row references it.
	if n, err := svc.SweepContents(ctx); err != nil || n != 0 {
		t.Fatalf("second sweep: %d %v", n, err)
	}
}

// Warnings come once per upward crossing, and again after the usage went
// below and crossed again.
func TestStorageWarnsOncePerCrossing(t *testing.T) {
	svc, _, w, by := newFilesService(t)
	ctx := context.Background()
	ch := ids.V7{}.New()
	if _, err := svc.SetQuota(ctx, by, 100); err != nil {
		t.Fatal(err)
	}
	up := func(n int, b byte) files.File {
		t.Helper()
		f, err := svc.Upload(ctx, by, ch, "x", bytes.NewReader(bytes.Repeat([]byte{b}, n)))
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	a := up(81, 'a')
	up(1, 'b')
	c := up(14, 'c') // 96
	if _, err := svc.Upload(ctx, by, ch, "y", bytes.NewReader(bytes.Repeat([]byte{'d'}, 5))); !errors.Is(err, files.ErrStorageFull) {
		t.Fatalf("over the quota: %v", err)
	}
	up(4, 'e')                                        // 100: full
	if err := svc.Delete(ctx, by, c.ID); err != nil { // 86
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, by, a.ID); err != nil { // 5
		t.Fatal(err)
	}
	up(80, 'f') // 85: warn again
	want := []files.StorageState{files.StorageWarning80, files.StorageWarning95, files.StorageFull, files.StorageWarning80}
	if len(w.seen) != len(want) {
		t.Fatalf("warnings = %v, want %v", w.seen, want)
	}
	for i := range want {
		if w.seen[i] != want[i] {
			t.Fatalf("warnings = %v, want %v", w.seen, want)
		}
	}
}

// Only the owner sets the quota; it is never negative.
func TestOnlyTheOwnerSetsTheQuota(t *testing.T) {
	svc, _, _, owner := newFilesService(t)
	ctx := context.Background()
	admin := owner
	admin.Role = identity.RoleAdmin
	if _, err := svc.SetQuota(ctx, admin, 10); !errors.Is(err, files.ErrNotOwner) {
		t.Fatalf("admin: %v", err)
	}
	if _, err := svc.SetQuota(ctx, owner, -1); !errors.Is(err, files.ErrInvalidQuota) {
		t.Fatalf("negative: %v", err)
	}
	if _, err := svc.StorageUsage(ctx, identity.Actor{WorkspaceID: owner.WorkspaceID, Kind: identity.KindHuman, Role: identity.RoleMember}); !errors.Is(err, files.ErrNotAdmin) {
		t.Fatalf("member reads usage: %v", err)
	}
}

func TestUsageLevels(t *testing.T) {
	for _, c := range []struct {
		used, quota int64
		want        files.StorageState
	}{
		{0, 0, files.StorageOK}, {1 << 40, 0, files.StorageOK}, {79, 100, files.StorageOK},
		{80, 100, files.StorageWarning80}, {94, 100, files.StorageWarning80}, {95, 100, files.StorageWarning95},
		{99, 100, files.StorageWarning95}, {100, 100, files.StorageFull}, {150, 100, files.StorageFull},
		{files.MaxQuotaBytes - 1, files.MaxQuotaBytes, files.StorageWarning95},
	} {
		u := files.Usage{FilesBytes: c.used, QuotaBytes: c.quota}
		if got := u.State(); got != c.want {
			t.Errorf("%d/%d = %s, want %s", c.used, c.quota, got, c.want)
		}
	}
	b := files.Usage{FilesBytes: 10, DBBytes: 30, ChatBytes: 20}.Breakdown()
	if b.Attachments != 10 || b.ChatSearch != 20 || b.Other != 10 {
		t.Fatalf("breakdown = %+v", b)
	}
}
