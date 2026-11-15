// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package secretbox

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func mustKey(t *testing.T, id string) string {
	t.Helper()
	k, err := NewKey(id)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSealOpenAndBinding(t *testing.T) {
	k1 := mustKey(t, "k1")
	ring, err := Parse(k1)
	if err != nil {
		t.Fatal(err)
	}
	aad := AAD("operator", "row-1", "", "totp")
	s, err := ring.Seal([]byte("JBSWY3DPEHPK3PXP"), aad)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(s.Ciphertext, []byte("JBSWY3DPEHPK3PXP")) {
		t.Fatal("the ciphertext contains the plaintext")
	}
	got, err := ring.Open(s, aad)
	if err != nil || string(got) != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("open: %q %v", got, err)
	}
	// Another row, another workspace or another kind does not open.
	for _, other := range [][]byte{AAD("operator", "row-2", "", "totp"), AAD("operator", "row-1", "ws", "totp"), AAD("operator", "row-1", "", "api_key")} {
		if _, err := ring.Open(s, other); !errors.Is(err, ErrOpen) {
			t.Fatalf("a value moved to %q opened: %v", other, err)
		}
	}
	// The parts are length-prefixed: "ab"+"c" is not "a"+"bc".
	if bytes.Equal(AAD("ab", "c"), AAD("a", "bc")) {
		t.Fatal("AAD is ambiguous")
	}
	// A changed ciphertext does not open.
	s.Ciphertext[0] ^= 1
	if _, err := ring.Open(s, aad); !errors.Is(err, ErrOpen) {
		t.Fatalf("a changed ciphertext opened: %v", err)
	}
	// Another keyring does not open it.
	s.Ciphertext[0] ^= 1
	other, _ := Parse(mustKey(t, "k1"))
	if _, err := other.Open(s, aad); !errors.Is(err, ErrOpen) {
		t.Fatalf("another key opened: %v", err)
	}
	// Two seals of the same value differ (fresh nonce).
	s2, _ := ring.Seal([]byte("JBSWY3DPEHPK3PXP"), aad)
	if bytes.Equal(s.Nonce, s2.Nonce) || bytes.Equal(s.Ciphertext, s2.Ciphertext) {
		t.Fatal("the nonce repeated")
	}
}

func TestRotation(t *testing.T) {
	k1, k2 := mustKey(t, "k1"), mustKey(t, "k2")
	old, _ := Parse(k1)
	aad := AAD("t", "r")
	s, _ := old.Seal([]byte("secret"), aad)

	ring, err := Parse(k2 + "," + k1)
	if err != nil {
		t.Fatal(err)
	}
	if ring.Active() != "k2" || !ring.NeedsRotation(s) {
		t.Fatalf("active %q, needs rotation %v", ring.Active(), ring.NeedsRotation(s))
	}
	plain, err := ring.Open(s, aad)
	if err != nil {
		t.Fatal(err)
	}
	re, _ := ring.Seal(plain, aad)
	// After the rotation, the new key alone opens it.
	only, _ := Parse(k2)
	if got, err := only.Open(re, aad); err != nil || string(got) != "secret" {
		t.Fatalf("rotated value: %q %v", got, err)
	}
	if _, err := only.Open(s, aad); !errors.Is(err, ErrNoKey) {
		t.Fatalf("the old value needs the old key: %v", err)
	}
}

func TestParseRejectsBadKeys(t *testing.T) {
	for _, bad := range []string{"", "k1", "k1:short", "K1:" + strings.Repeat("A", 44), mustKey(t, "k1") + "," + mustKey(t, "k1")} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
