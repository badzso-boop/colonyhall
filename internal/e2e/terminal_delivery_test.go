package e2e_test

import (
	"context"
	"testing"

	"colonyhall/internal/modules/delivery"
	deliverymemory "colonyhall/internal/modules/delivery/memory"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

func TestTerminalFinishPathPostsMessageAndWakeItem(t *testing.T) {
	memStore := deliverymemory.New()
	idGen := &ids.V7{}
	svc := &delivery.Service{
		Store: memStore,
		IDs:   idGen,
		Clock: &clock.System{},
	}

	agentActor := identity.Actor{
		ID:          idGen.New(),
		WorkspaceID: idGen.New(),
		Kind:        identity.KindAgent,
		Handle:      "szakdoga",
	}

	chID := idGen.New()
	thRootID := idGen.New()

	exit0 := 0
	err := svc.CreateTerminalEvent(
		context.Background(),
		agentActor,
		"ros-build",
		chID,
		thRootID,
		&exit0,
		2592000, // ~43 min
		"line 1\nline 2\nBuild succeeded",
	)
	if err != nil {
		t.Fatalf("CreateTerminalEvent failed: %v", err)
	}

	// Verify exactly one wake item created in delivery
	ds, total, err := memStore.Pending(context.Background(), agentActor.ID, 10)
	if err != nil {
		t.Fatalf("Pending failed: %v", err)
	}
	if total != 1 || len(ds) != 1 {
		t.Fatalf("expected exactly 1 wake delivery, got total=%d len=%d", total, len(ds))
	}
	if ds[0].ItemKind != delivery.ItemTerminal {
		t.Errorf("expected ItemKind 'terminal', got %q", ds[0].ItemKind)
	}
	if !ds[0].Flags.Has(delivery.FlagWake) {
		t.Errorf("expected delivery to have FlagWake")
	}
}
