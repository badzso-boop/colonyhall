// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory implements in-memory adapters for the files module.
package memory

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"

	"colonyhall/internal/modules/files"
)

// Storage is an in-memory StorageBackend for tests and memory mode.
type Storage struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewStorage creates an empty in-memory storage backend.
func NewStorage() *Storage {
	return &Storage{data: make(map[string][]byte)}
}

// Put writes data into memory map.
func (s *Storage) Put(_ context.Context, sha256Hex string, r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sha256Hex] = b
	return int64(len(b)), nil
}

type nopCloserReadSeeker struct {
	*bytes.Reader
}

func (nopCloserReadSeeker) Close() error { return nil }

// Open opens a reader for content stored under sha256Hex.
func (s *Storage) Open(_ context.Context, sha256Hex string) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.data[sha256Hex]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &nopCloserReadSeeker{Reader: bytes.NewReader(b)}, nil
}

// Stat returns the length of content stored under sha256Hex.
func (s *Storage) Stat(_ context.Context, sha256Hex string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.data[sha256Hex]
	if !ok {
		return 0, os.ErrNotExist
	}
	return int64(len(b)), nil
}

var _ files.StorageBackend = (*Storage)(nil)
