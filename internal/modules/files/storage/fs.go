// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package storage implements storage backend adapters for the files module.
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"colonyhall/internal/modules/files"
)

// FSStorage implements files.StorageBackend on the local filesystem.
type FSStorage struct {
	dir string
}

// NewFS creates a new filesystem storage backend in dir.
func NewFS(dir string) (*FSStorage, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("storage: create files dir %q: %w", dir, err)
	}
	return &FSStorage{dir: dir}, nil
}

// Put writes data from r to dir/<sha256Hex> atomically.
func (s *FSStorage) Put(ctx context.Context, sha256Hex string, r io.Reader) (int64, error) {
	target := filepath.Join(s.dir, sha256Hex)
	// Check if already exists (deduplication)
	if fi, err := os.Stat(target); err == nil {
		return fi.Size(), nil
	}

	// Write to a temporary file in the same directory first
	var randBytes [8]byte
	_, _ = rand.Read(randBytes[:])
	tmp := filepath.Join(s.dir, fmt.Sprintf(".tmp-%s-%s", sha256Hex, hex.EncodeToString(randBytes[:])))

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}

	n, err := io.Copy(f, r)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return 0, err
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return 0, err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}

	// Rename atomically to final content-addressed location
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		// If another concurrent upload won the race, it's fine
		if fi, statErr := os.Stat(target); statErr == nil {
			return fi.Size(), nil
		}
		return 0, err
	}

	return n, nil
}

// Open opens the file identified by sha256Hex for reading.
func (s *FSStorage) Open(ctx context.Context, sha256Hex string) (io.ReadCloser, error) {
	target := filepath.Join(s.dir, sha256Hex)
	return os.Open(target)
}

// Stat returns the byte size of the file identified by sha256Hex.
func (s *FSStorage) Stat(ctx context.Context, sha256Hex string) (int64, error) {
	target := filepath.Join(s.dir, sha256Hex)
	fi, err := os.Stat(target)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

var _ files.StorageBackend = (*FSStorage)(nil)
