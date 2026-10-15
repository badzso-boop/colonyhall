package conversation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// MaxDMParticipants bounds a group DM (the caller plus this many others).
const MaxDMParticipants = 8

// dmName derives a stable channel name from the member set, so opening the
// same DM twice finds the existing channel instead of creating another.
func dmName(members []ids.ID) string {
	sorted := append([]ids.ID(nil), members...)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i][:], sorted[j][:]) < 0 })
	h := sha256.New()
	for _, id := range sorted {
		h.Write(id[:])
	}
	return dmPrefix + hex.EncodeToString(h.Sum(nil))[:16]
}

// OpenDM returns the DM channel between the caller and others, creating it
// on first use. Everyone in it may write; admins can read every DM.
func (s *Service) OpenDM(ctx context.Context, by identity.Actor, others []ids.ID) (Channel, error) {
	if by.Halted {
		return Channel{}, identity.ErrHalted
	}
	seen := map[ids.ID]bool{by.ID: true}
	members := []ids.ID{by.ID}
	for _, id := range others {
		if !seen[id] {
			seen[id] = true
			members = append(members, id)
		}
	}
	if len(members) < 2 || len(members) > MaxDMParticipants+1 {
		return Channel{}, ErrBadDM
	}
	name := dmName(members)
	var c Channel
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		existing, err := s.Store.ChannelByName(ctx, by.WorkspaceID, name)
		if err == nil {
			// Never hand out a channel that merely has the name: it must be a
			// DM with exactly these members (guards against squatting and
			// against a hash-prefix collision).
			ok, err := s.sameMembers(ctx, existing, members)
			if err != nil {
				return err
			}
			if !ok {
				return ErrBadDM
			}
			c = existing
			return nil
		}
		if !errors.Is(err, db.ErrNotFound) {
			return err
		}
		for _, id := range members[1:] {
			a, err := s.Directory.ActorByID(ctx, id)
			if err != nil || a.WorkspaceID != by.WorkspaceID {
				return ErrBadDM
			}
		}
		c = Channel{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, Kind: KindDM, Name: name, CreatedBy: by.ID, CreatedAt: s.Clock.Now()}
		if err := s.Store.CreateChannel(ctx, c); err != nil {
			return err
		}
		if seq, err = s.appendEvent(ctx, by, c.ID, EventChannelCreated, "channel", c.ID,
			ChannelCreated{ChannelID: c.ID.String(), Kind: KindDM, Name: name}); err != nil {
			return err
		}
		for _, id := range members {
			if seq, err = s.addMember(ctx, by, c, id, true); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return c, nil
}

// OpenDMByHandle opens a DM with one person by @handle.
func (s *Service) OpenDMByHandle(ctx context.Context, by identity.Actor, handle string) (Channel, error) {
	a, err := s.Directory.ActorByHandle(ctx, by.WorkspaceID, handle)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Channel{}, ErrBadDM
		}
		return Channel{}, err
	}
	return s.OpenDM(ctx, by, []ids.ID{a.ID})
}

func (s *Service) sameMembers(ctx context.Context, c Channel, members []ids.ID) (bool, error) {
	if c.Kind != KindDM {
		return false, nil
	}
	stored, err := s.Store.MemberIDs(ctx, c.ID)
	if err != nil {
		return false, err
	}
	if len(stored) != len(members) {
		return false, nil
	}
	want := map[ids.ID]bool{}
	for _, id := range members {
		want[id] = true
	}
	for _, id := range stored {
		if !want[id] {
			return false, nil
		}
	}
	return true, nil
}
