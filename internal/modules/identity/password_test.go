// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword("correct horse battery", h); err != nil || !ok {
		t.Fatalf("right password rejected: %v", err)
	}
	if ok, _ := VerifyPassword("wrong horse battery", h); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h == h2 {
		t.Fatal("hashes must be salted")
	}
	if _, err := VerifyPassword("x", "$argon2id$garbage"); err == nil {
		t.Fatal("malformed hash must error")
	}
}

func TestPasswordPolicy(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Fatal("short password accepted")
	}
	if ValidatePassword("long-enough-password") != nil {
		t.Fatal("valid password rejected")
	}
}
