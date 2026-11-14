// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func TestFSStorage(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFS(dir)
	if err != nil {
		t.Fatalf("NewFS failed: %v", err)
	}

	content := []byte("hello storage world")
	hash := sha256.Sum256(content)
	hashHex := hex.EncodeToString(hash[:])

	ctx := context.Background()

	// Put
	n, err := fs.Put(ctx, hashHex, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("Put returned size %d, want %d", n, len(content))
	}

	// Put deduplication (calling again should succeed)
	n2, err := fs.Put(ctx, hashHex, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("second Put failed: %v", err)
	}
	if n2 != int64(len(content)) {
		t.Fatalf("second Put returned size %d, want %d", n2, len(content))
	}

	// Stat
	sz, err := fs.Stat(ctx, hashHex)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if sz != int64(len(content)) {
		t.Fatalf("Stat size %d, want %d", sz, len(content))
	}

	// Open
	rc, err := fs.Open(ctx, hashHex)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	readBytes, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(readBytes, content) {
		t.Fatalf("got %q, want %q", string(readBytes), string(content))
	}

	// Stat nonexistent
	if _, err := fs.Stat(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent stat, got nil")
	}

	// Open nonexistent
	if _, err := fs.Open(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent open, got nil")
	}
}
