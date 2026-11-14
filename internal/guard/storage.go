package guard

import (
	"log/slog"
	"sync"
)

// storageFull is the host config's storage state that stops wake-ups.
const storageFull = "full"

// HostStorage is the workspace storage in the host config (round 54);
// absent from older servers (nil: nothing changes).
type HostStorage struct {
	State      string `json:"state"`
	UsedBytes  int64  `json:"used_bytes,omitempty"`
	QuotaBytes int64  `json:"quota_bytes,omitempty"`
}

// StorageGate holds the workspace storage state for every agent of the
// machine (round 54). While it is full the agents start no runs: an agent
// could not upload its results, and every wake-up would spend its AI quota
// for nothing. The items stay in the inbox and run when there is room
// again; a run already going is never stopped. Like the Claude usage pause,
// it is logged once per change, not per item.
type StorageGate struct {
	mu   sync.Mutex
	full bool
	log  *slog.Logger
	// onChange runs (in its own goroutine) after the state changed.
	onChange func(full bool)
}

// NewStorageGate returns an open gate.
func NewStorageGate(log *slog.Logger, onChange func(full bool)) *StorageGate {
	if log == nil {
		log = slog.Default()
	}
	return &StorageGate{log: log, onChange: onChange}
}

// Full reports whether runs must wait; a nil gate never stops them.
func (g *StorageGate) Full() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.full
}

// Set applies the host config's storage; nil (an older server) is not full.
func (g *StorageGate) Set(st *HostStorage) {
	if g == nil {
		return
	}
	full := st != nil && st.State == storageFull
	g.mu.Lock()
	changed := full != g.full
	g.full = full
	g.mu.Unlock()
	if !changed {
		return
	}
	if full {
		g.log.Warn("guard: paused: workspace storage full; agents are not woken until there is room",
			"used_bytes", st.UsedBytes, "quota_bytes", st.QuotaBytes)
	} else {
		g.log.Info("guard: workspace storage has room again; resuming")
	}
	if g.onChange != nil {
		go g.onChange(full)
	}
}
