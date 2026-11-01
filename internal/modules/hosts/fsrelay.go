package hosts

import (
	"context"
	"errors"
	"sync"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Host folder listing relay (round 33): an admin's folder picker asks a
// guard to list folders inside its guard.yaml roots before any agent exists
// on the machine. The request lives only in memory; the guard learns about
// it over its host WebSocket (/ws/host), fetches it and posts the answer
// with its host token.

// EventFsRequested records that an admin listed folders on a host. The
// payload never carries the path.
const EventFsRequested = "host.fs_requested"

// Host fs operations.
const OpListDirs = "list_dirs"

// DefaultFsTimeout is how long an admin waits for the guard.
const DefaultFsTimeout = 15 * time.Second

// MaxDirEntries bounds one listing (the guard truncates at the same size).
const MaxDirEntries = 500

// Relay errors.
var (
	// ErrHostOffline: the guard holds no host WebSocket. The text is the
	// problem detail clients match on.
	ErrHostOffline = errors.New("host_offline")
	ErrFsTimeout   = errors.New("the machine did not answer in time")
	ErrFsGone      = errors.New("the request expired")
	ErrBadFsOp     = errors.New("op must be list_dirs")
	ErrTooMany     = errors.New("at most 500 entries per listing")
)

// FsEvent is the audit payload of EventFsRequested.
type FsEvent struct {
	HostID string `json:"host_id"`
	Op     string `json:"op"`
}

// HostFsPusher wakes the guard of a host (the gateway's /ws/host). It
// reports false when no connection of that host took the frame.
type HostFsPusher interface {
	PushHostFsRequest(hostID ids.ID, requestID string) bool
}

// FsPending is a request waiting for its guard.
type FsPending struct {
	ID     string
	HostID ids.ID
	Op     string
	Path   string
	result chan apispec.HostDirListing
}

// FsRelay relays host fs requests to guards. It is safe for concurrent use.
type FsRelay struct {
	Hosts   *Service
	Pusher  HostFsPusher
	Timeout time.Duration // DefaultFsTimeout when zero

	mu      sync.Mutex
	pending map[string]*FsPending
	expired map[string]time.Time // answered or timed-out ids, for 410
}

// ListDirs asks the host's guard for the folders inside path (empty: its
// roots) and waits for the answer.
func (r *FsRelay) ListDirs(ctx context.Context, by identity.Actor, hostID ids.ID, path string) (apispec.HostDirListing, error) {
	if err := admin(by); err != nil {
		return apispec.HostDirListing{}, err
	}
	if path != "" {
		if err := ValidatePath(path); err != nil {
			return apispec.HostDirListing{}, err
		}
	}
	h, err := r.Hosts.hostIn(ctx, by.WorkspaceID, hostID)
	if err != nil {
		return apispec.HostDirListing{}, err
	}
	if h.State != StateApproved {
		return apispec.HostDirListing{}, ErrHostOffline
	}
	if err := r.Hosts.record(ctx, by.WorkspaceID, by.ID, EventFsRequested, h.ID,
		FsEvent{HostID: h.ID.String(), Op: OpListDirs}, func(context.Context) error { return nil }); err != nil {
		return apispec.HostDirListing{}, err
	}

	p := &FsPending{ID: r.Hosts.IDs.New().String(), HostID: h.ID, Op: OpListDirs, Path: path,
		result: make(chan apispec.HostDirListing, 1)}
	r.mu.Lock()
	if r.pending == nil {
		r.pending = map[string]*FsPending{}
		r.expired = map[string]time.Time{}
	}
	r.pending[p.ID] = p
	r.mu.Unlock()

	if r.Pusher == nil || !r.Pusher.PushHostFsRequest(h.ID, p.ID) {
		r.finish(p.ID)
		return apispec.HostDirListing{}, ErrHostOffline
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultFsTimeout
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case res := <-p.result:
		return res, nil
	case <-t.C:
		r.finish(p.ID)
		return apispec.HostDirListing{}, ErrFsTimeout
	case <-ctx.Done():
		r.finish(p.ID)
		return apispec.HostDirListing{}, ctx.Err()
	}
}

// Request returns a pending request of host h; another host's is not found.
func (r *FsRelay) Request(h Host, id string) (FsPending, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok || p.HostID != h.ID {
		return FsPending{}, db.ErrNotFound
	}
	return FsPending{ID: p.ID, HostID: p.HostID, Op: p.Op, Path: p.Path}, nil
}

// Deliver hands the guard's answer to the waiting admin.
func (r *FsRelay) Deliver(h Host, id string, res apispec.HostDirListing) error {
	if len(res.Entries) > MaxDirEntries {
		return ErrTooMany
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok {
		if _, gone := r.expired[id]; gone {
			return ErrFsGone
		}
		return db.ErrNotFound
	}
	if p.HostID != h.ID {
		return db.ErrNotFound
	}
	r.finishLocked(id)
	if res.Entries == nil {
		res.Entries = []apispec.HostDirEntry{}
	}
	p.result <- res
	return nil
}

func (r *FsRelay) finish(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked(id)
}

func (r *FsRelay) finishLocked(id string) {
	if _, ok := r.pending[id]; !ok {
		return
	}
	delete(r.pending, id)
	now := r.Hosts.Clock.Now()
	r.expired[id] = now
	for k, at := range r.expired {
		if now.Sub(at) > 5*time.Minute {
			delete(r.expired, k)
		}
	}
}
