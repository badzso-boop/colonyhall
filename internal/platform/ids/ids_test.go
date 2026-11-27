// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package ids

import "testing"

func TestNilAsNull(t *testing.T) {
	if p := NilAsNull(Nil); p != nil {
		t.Fatalf("NilAsNull(Nil) = %v, want nil", *p)
	}
	id := (&Sequential{}).New()
	p := NilAsNull(id)
	if p == nil || *p != id {
		t.Fatalf("NilAsNull(%s) = %v", id, p)
	}
	id[0] ^= 0xff
	if *p == id {
		t.Fatal("NilAsNull returned a pointer to the caller's variable")
	}
}
