// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package ids provides the IDGenerator port. Production uses time-ordered
// UUIDv7; tests use a deterministic sequential generator.
package ids

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// ID is the identifier type used for every entity.
type ID = uuid.UUID

// Nil is the zero ID.
var Nil = uuid.Nil

// Generator creates new IDs.
type Generator interface {
	New() ID
}

// V7 generates time-ordered UUIDv7 identifiers.
type V7 struct{}

// New returns a new UUIDv7. It panics only if the system random source fails.
func (V7) New() ID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("ids: cannot generate UUIDv7: %v", err))
	}
	return id
}

// Sequential returns predictable IDs (…0001, …0002, …) for tests.
type Sequential struct {
	mu sync.Mutex
	n  uint64
}

// New returns the next sequential ID.
func (s *Sequential) New() ID {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	var id ID
	for i := 0; i < 8; i++ {
		id[15-i] = byte(s.n >> (8 * i))
	}
	return id
}

// Parse parses an ID from its string form.
func Parse(s string) (ID, error) { return uuid.Parse(s) }

// NilAsNull returns nil for the zero ID and a pointer to id otherwise: the
// form of a nullable UUID column.
func NilAsNull(id ID) *ID {
	if id == Nil {
		return nil
	}
	return &id
}
