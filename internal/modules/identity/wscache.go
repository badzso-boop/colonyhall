package identity

import (
	"sync"

	"colonyhall/internal/platform/ids"
)

// workspaceCache remembers which workspace a token or session belongs to
// (round 45). That never changes, so a hit is always right; whether the
// credential is still valid is checked by the scoped query that follows.
// It saves the lookup's round trip on every request. Bounded: it starts
// over when full.
type workspaceCache struct {
	mu sync.Mutex
	m  map[string]ids.ID
}

const workspaceCacheMax = 10000

func (c *workspaceCache) get(kind string, hash []byte) (ids.ID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ws, ok := c.m[kind+string(hash)]
	return ws, ok
}

func (c *workspaceCache) put(kind string, hash []byte, ws ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= workspaceCacheMax {
		c.m = make(map[string]ids.ID)
	}
	c.m[kind+string(hash)] = ws
}
