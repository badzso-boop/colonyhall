// Package audit keeps a tamper-evident copy of the event log. Every entry
// stores the hash of the previous entry of the same workspace, so altering
// or deleting a row breaks the chain, which Verify detects.
package audit

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"colonyhall/internal/platform/ids"
)

// Entry is one audit record; Seq is the event's sequence number.
type Entry struct {
	Seq         int64
	WorkspaceID ids.ID
	ActorID     ids.ID
	Action      string
	TargetType  string
	TargetID    ids.ID
	Payload     []byte
	At          time.Time
	PrevHash    []byte
	Hash        []byte
}

// ComputeHash returns the entry's hash over its content and PrevHash.
// Fields are length-prefixed so different splits cannot collide.
func (e Entry) ComputeHash() []byte {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], uint64(e.Seq))
	write(e.PrevHash)
	write(seq[:])
	write(e.WorkspaceID[:])
	write(e.ActorID[:])
	write([]byte(e.Action))
	write([]byte(e.TargetType))
	write(e.TargetID[:])
	// Postgres stores microseconds; hash at that precision so a stored entry
	// verifies the same as the freshly built one.
	write([]byte(e.At.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)))
	write(e.Payload)
	return h.Sum(nil)
}

// VerifyResult reports the outcome of a chain check.
type VerifyResult struct {
	OK       bool
	Checked  int
	BrokenAt int64 // seq of the first bad entry, 0 when OK
}

// Limits.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)
