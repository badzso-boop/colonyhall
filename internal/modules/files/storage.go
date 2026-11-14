// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files

import (
	"context"
	"io"
)

// StorageBackend stores content addressed files by their SHA256 hex hash.
// Content is immutable; it is removed only when no live file row references
// its hash any more (round 54: the sweep, under the content lock).
type StorageBackend interface {
	// Put writes data from r under sha256Hex, returning written bytes.
	// If the file already exists, it is idempotent.
	Put(ctx context.Context, sha256Hex string, r io.Reader) (int64, error)

	// Open returns a reader for the file content with the given sha256Hex.
	// Returns os.ErrNotExist if not found.
	Open(ctx context.Context, sha256Hex string) (io.ReadCloser, error)

	// Stat returns the byte size of the file with the given sha256Hex.
	// Returns os.ErrNotExist if not found.
	Stat(ctx context.Context, sha256Hex string) (int64, error)

	// Delete removes the content; a missing one is not an error.
	Delete(ctx context.Context, sha256Hex string) error
}
