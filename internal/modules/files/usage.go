package files

import (
	"context"
	"errors"
	"io"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// OwnedAgents lists the agents a person owns (round 38's actor.owner_id),
// archived ones included: their files stay the person's to delete.
type OwnedAgents interface {
	AgentIDsOwnedBy(ctx context.Context, workspaceID, ownerID ids.ID) ([]ids.ID, error)
}

// StorageNotifier tells the workspace owner that the storage crossed a
// warning level upwards (80%, 95%, full). It names numbers only, never
// people or files. Called after the change is committed.
type StorageNotifier interface {
	StorageWarning(ctx context.Context, workspaceID ids.ID, u Usage)
}

// newFile is a row and its content, stored together by storeNew.
type newFile struct {
	file    File
	content io.Reader
	// extra runs in the same transaction after the row is inserted.
	extra func(ctx context.Context) error
	// The event recorded with it.
	evType    string
	evChannel ids.ID
	evAgg     ids.ID
	evPayload any
}

// storeNew writes a file's content and row in one transaction, after the
// quota check. The content lock (per sha256, every server) keeps the sweep
// from removing the content between the write and the row: the sweep takes
// the same lock and checks for a live row before it deletes. The usage row
// lock serializes the workspace's uploads, so a burst cannot overshoot the
// quota.
func (s *Service) storeNew(ctx context.Context, actorID ids.ID, n newFile) error {
	f := n.file
	now := f.CreatedAt
	var seq int64
	var warn *Usage
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.LockContent(ctx, f.SHA256); err != nil {
			return err
		}
		u, err := s.Store.LockUsage(ctx, f.WorkspaceID, now)
		if err != nil {
			return err
		}
		had, err := s.Store.WorkspaceHasLiveContent(ctx, f.WorkspaceID, f.SHA256)
		if err != nil {
			return err
		}
		live, err := s.Store.LiveFilesBytes(ctx, f.WorkspaceID)
		if err != nil {
			return err
		}
		if !had {
			// A copy of content the workspace already has adds nothing.
			if u.QuotaBytes > 0 && live+f.Size+u.DBBytes > u.QuotaBytes {
				return &FullError{Used: live + u.DBBytes, Quota: u.QuotaBytes}
			}
			live += f.Size
		}
		if _, err := s.Storage.Put(ctx, f.SHA256, n.content); err != nil {
			return err
		}
		if err := s.Store.Insert(ctx, f); err != nil {
			return err
		}
		if n.extra != nil {
			if err := n.extra(ctx); err != nil {
				return err
			}
		}
		if s.Events != nil {
			ev, err := eventlog.NewEvent(s.IDs.New(), f.WorkspaceID, n.evType, actorID, n.evChannel, "files", n.evAgg, n.evPayload, now)
			if err != nil {
				return err
			}
			if err := s.Events.Append(ctx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		u.FilesBytes = live
		sseq, w, err := s.saveUsage(ctx, &u, actorID, now)
		if err != nil {
			return err
		}
		seq = max(seq, sseq)
		warn = w
		return nil
	})
	if err != nil {
		return err
	}
	s.afterUsage(ctx, seq, warn)
	return nil
}

// saveUsage stores u and, when its level changed, records the state change
// (audited; it also reaches the guards: a workspace.* event). It returns the
// usage to warn the owner about when the level went up to 80% or more.
func (s *Service) saveUsage(ctx context.Context, u *Usage, actorID ids.ID, now time.Time) (int64, *Usage, error) {
	prev := u.WarnedLevel
	level := u.Level()
	u.WarnedLevel = level
	u.UpdatedAt = now
	if err := s.Store.SaveUsage(ctx, *u); err != nil {
		return 0, nil, err
	}
	if level == prev {
		return 0, nil, nil
	}
	var seq int64
	if s.Events != nil {
		ev, err := eventlog.NewEvent(s.IDs.New(), u.WorkspaceID, EventStorageStateChanged, actorID, ids.Nil, "workspace", u.WorkspaceID,
			StorageStatePayload{State: string(stateOf(level)), PreviousState: string(stateOf(prev)), UsedBytes: u.Used(), QuotaBytes: u.QuotaBytes}, now)
		if err != nil {
			return 0, nil, err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return 0, nil, err
		}
		seq = ev.Seq
	}
	if level > prev && level >= Level80 {
		w := *u
		return seq, &w, nil
	}
	return seq, nil, nil
}

func (s *Service) afterUsage(ctx context.Context, seq int64, warn *Usage) {
	if seq > 0 && s.Bus != nil {
		s.Bus.Notify(seq)
	}
	if warn != nil && s.Notifier != nil {
		s.Notifier.StorageWarning(ctx, warn.WorkspaceID, *warn)
	}
}

// Delete soft-deletes a file. Only its uploader may, or the owner of the
// agent that uploaded or fetched it; nobody else, the workspace owner
// included (Norbi, 2026-11-14). Messages that carried it show "File
// deleted"; the content leaves the store when the sweep finds no live row
// with its hash.
func (s *Service) Delete(ctx context.Context, by identity.Actor, fileID ids.ID) error {
	if by.Halted {
		return identity.ErrHalted
	}
	f, err := s.Store.ByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f.WorkspaceID != by.WorkspaceID {
		return ErrNotFound
	}
	if f.DeletedAt != nil {
		return ErrGone
	}
	if f.UploadedBy != by.ID {
		mine := false
		if by.Kind == identity.KindHuman && s.Actors != nil {
			up, err := s.Actors.ActorByID(ctx, f.UploadedBy)
			if err != nil {
				return err
			}
			mine = up.Kind == identity.KindAgent && up.OwnerID == by.ID
		}
		if !mine {
			return ErrNotUploader
		}
	}
	now := s.Clock.Now()
	var seq int64
	var warn *Usage
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		u, err := s.Store.LockUsage(ctx, f.WorkspaceID, now)
		if err != nil {
			return err
		}
		if err := s.Store.MarkDeleted(ctx, f.ID, now); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrGone
			}
			return err
		}
		if s.Events != nil {
			// No name, no size: the uploader may want the name gone too.
			ev, err := eventlog.NewEvent(s.IDs.New(), f.WorkspaceID, EventFileDeleted, by.ID, f.ChannelID, "files", f.ID,
				DeletedPayload{ID: f.ID.String(), ChannelID: f.ChannelID.String()}, now)
			if err != nil {
				return err
			}
			if err := s.Events.Append(ctx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		if u.FilesBytes, err = s.Store.LiveFilesBytes(ctx, f.WorkspaceID); err != nil {
			return err
		}
		sseq, w, err := s.saveUsage(ctx, &u, by.ID, now)
		seq, warn = max(seq, sseq), w
		return err
	})
	if err != nil {
		return err
	}
	s.afterUsage(ctx, seq, warn)
	return nil
}

// MyFilesChannel is the channel a listed file is in, when the person can
// still read it.
type MyFilesChannel struct {
	ID   ids.ID
	Name string
	Kind string
}

// MyFileView is a "My files" row as the person may see it: the channel and
// the message only when they can still read the channel.
type MyFileView struct {
	MyFile
	Channel *MyFilesChannel
}

// ListMine lists the person's files (UploaderMe) or their agents' files
// (UploaderAgents), newest first. Only humans have "My files".
func (s *Service) ListMine(ctx context.Context, by identity.Actor, q MyFilesQuery, who Uploader) ([]MyFileView, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	q.WorkspaceID = by.WorkspaceID
	switch who {
	case UploaderAgents:
		if s.Agents == nil {
			return []MyFileView{}, nil
		}
		agentIDs, err := s.Agents.AgentIDsOwnedBy(ctx, by.WorkspaceID, by.ID)
		if err != nil {
			return nil, err
		}
		if len(agentIDs) == 0 {
			return []MyFileView{}, nil
		}
		q.UploaderIDs = agentIDs
	default:
		q.UploaderIDs = []ids.ID{by.ID}
	}
	if q.Limit <= 0 {
		q.Limit = DefaultMyFilesLimit
	}
	q.Limit = min(q.Limit, MaxMyFilesLimit)
	rows, err := s.Store.ListMine(ctx, q)
	if err != nil {
		return nil, err
	}
	readable := map[ids.ID]*MyFilesChannel{}
	out := make([]MyFileView, 0, len(rows))
	for _, r := range rows {
		ch, known := readable[r.ChannelID]
		if !known {
			ch = nil
			if ok, err := s.Conversation.CanRead(ctx, by, r.ChannelID); err != nil {
				return nil, err
			} else if ok {
				c, err := s.Conversation.ChannelByID(ctx, r.ChannelID)
				if err != nil {
					return nil, err
				}
				ch = &MyFilesChannel{ID: c.ID, Name: c.Name, Kind: string(c.Kind)}
			}
			readable[r.ChannelID] = ch
		}
		v := MyFileView{MyFile: r, Channel: ch}
		if ch == nil {
			// A channel the person left: no name, no link into it.
			v.MessageID, v.ThreadRootID = ids.Nil, ids.Nil
		} else if r.MessageID != ids.Nil {
			m, err := s.Conversation.MessageByID(ctx, r.MessageID)
			if err != nil && !errors.Is(err, db.ErrNotFound) {
				return nil, err
			}
			v.ThreadRootID = m.ThreadRootID
		}
		out = append(out, v)
	}
	return out, nil
}

// StorageUsage returns the workspace's storage for the owner and admins.
func (s *Service) StorageUsage(ctx context.Context, by identity.Actor) (Usage, error) {
	if !by.IsAdmin() || by.Kind != identity.KindHuman {
		return Usage{}, ErrNotAdmin
	}
	return s.Store.UsageOf(ctx, by.WorkspaceID)
}

// StorageState is the workspace's state for the guards' host config.
func (s *Service) StorageState(ctx context.Context, workspaceID ids.ID) (Usage, error) {
	return s.Store.UsageOf(ctx, workspaceID)
}

// SetQuota sets the workspace's quota in bytes (0: unlimited). Owner only.
func (s *Service) SetQuota(ctx context.Context, by identity.Actor, quotaBytes int64) (Usage, error) {
	if by.Role != identity.RoleOwner || by.Kind != identity.KindHuman {
		return Usage{}, ErrNotOwner
	}
	if quotaBytes < 0 || quotaBytes > MaxQuotaBytes {
		return Usage{}, ErrInvalidQuota
	}
	now := s.Clock.Now()
	var u Usage
	var seq int64
	var warn *Usage
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if u, err = s.Store.LockUsage(ctx, by.WorkspaceID, now); err != nil {
			return err
		}
		u.QuotaBytes = quotaBytes
		if u.FilesBytes, err = s.Store.LiveFilesBytes(ctx, by.WorkspaceID); err != nil {
			return err
		}
		if s.Events != nil {
			ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventStorageQuotaSet, by.ID, ids.Nil, "workspace", by.WorkspaceID,
				QuotaSetPayload{QuotaBytes: quotaBytes}, now)
			if err != nil {
				return err
			}
			if err := s.Events.Append(ctx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		sseq, w, err := s.saveUsage(ctx, &u, by.ID, now)
		seq, warn = max(seq, sseq), w
		return err
	})
	if err != nil {
		return Usage{}, err
	}
	s.afterUsage(ctx, seq, warn)
	return u, nil
}

// RefreshUsage stores the hourly database estimate and recounts the files
// (which the uploads and deletes already keep exact). ctx must be scoped to
// the workspace (db.WithWorkspace).
func (s *Service) RefreshUsage(ctx context.Context, workspaceID ids.ID, est DBEstimate) (Usage, error) {
	now := s.Clock.Now()
	var u Usage
	var seq int64
	var warn *Usage
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if u, err = s.Store.LockUsage(ctx, workspaceID, now); err != nil {
			return err
		}
		if u.FilesBytes, err = s.Store.LiveFilesBytes(ctx, workspaceID); err != nil {
			return err
		}
		u.DBBytes, u.ChatBytes = max(est.Total, 0), max(est.Chat, 0)
		at := est.At
		if at.IsZero() {
			at = now
		}
		u.DBMeasuredAt = &at
		seq, warn, err = s.saveUsage(ctx, &u, ids.Nil, now)
		return err
	})
	if err != nil {
		return Usage{}, err
	}
	s.afterUsage(ctx, seq, warn)
	return u, nil
}

// SweepContents removes stored content that no live file row references any
// more, in any workspace. It runs in system mode (the maintenance loop): a
// hash may be shared across workspaces. Each removal holds the content lock
// that uploads take, and checks for a live row under it, so a concurrent
// upload of the same content either commits its row first (the content
// stays) or waits and writes the content again after the removal.
func (s *Service) SweepContents(ctx context.Context) (removed int, err error) {
	hashes, err := s.Store.RemovableContents(ctx, SweepBatch)
	if err != nil {
		return 0, err
	}
	for _, h := range hashes {
		now := s.Clock.Now()
		err := s.Tx.InTx(ctx, func(ctx context.Context) error {
			if err := s.Store.LockContent(ctx, h); err != nil {
				return err
			}
			live, err := s.Store.ContentLive(ctx, h)
			if err != nil {
				return err
			}
			if !live {
				if err := s.Storage.Delete(ctx, h); err != nil {
					return err
				}
				removed++
			}
			return s.Store.MarkContentRemoved(ctx, h, now)
		})
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}
