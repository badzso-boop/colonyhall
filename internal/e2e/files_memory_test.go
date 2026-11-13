package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"runtime"
	"testing"

	filesstorage "colonyhall/internal/modules/files/storage"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// Round 40: an upload is staged on disk, not in memory, so parallel
// uploads fit the hardened container's memory limit. Reading a 20 MB
// upload into a growing buffer allocated well over 20 MB.
func TestUploadIsNotBufferedInMemory(t *testing.T) {
	e := newEnv(t)
	t.Setenv("TMPDIR", t.TempDir())
	// The memory store keeps files in memory; production writes to disk.
	fs, err := filesstorage.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.app.Files.Storage = fs
	ctx := context.Background()
	by, err := e.app.Identity.Authenticate(ctx, server.DevAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx = db.WithWorkspace(ctx, by.WorkspaceID)
	payload := make([]byte, 20<<20)
	_, _ = rand.Read(payload)
	general, err := ids.Parse(e.generalID())
	if err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f, err := e.app.Files.Upload(ctx, by, general, "big.bin", bytes.NewReader(payload))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 4<<20 {
		t.Fatalf("a 20 MB upload allocated %d MB", alloc>>20)
	}
	_, rc, err := e.app.Files.OpenContent(ctx, by, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, payload) || f.Size != int64(len(payload)) {
		t.Fatal("the stored file differs from the upload")
	}
}
