package identity

import (
	"context"
	"testing"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// lookupStore counts the workspace lookups of tokens.
type lookupStore struct {
	Store
	lookups int
	ws      ids.ID
}

func (s *lookupStore) WorkspaceOfToken(context.Context, []byte) (ids.ID, error) {
	s.lookups++
	return s.ws, nil
}

func TestCredentialWorkspaceIsLookedUpOnce(t *testing.T) {
	ws, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35a")
	st := &lookupStore{ws: ws}
	svc := &Service{Store: st}
	for range 3 {
		ctx, err := svc.credentialScope(context.Background(), credToken, st.WorkspaceOfToken, HashSecret("col_x"))
		if err != nil || db.ScopeOf(ctx).Workspace != ws {
			t.Fatalf("scope: %+v %v", db.ScopeOf(ctx), err)
		}
	}
	if st.lookups != 1 {
		t.Fatalf("the token's workspace never changes; looked up %d times", st.lookups)
	}
}
