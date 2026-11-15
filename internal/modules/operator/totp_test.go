// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA-1, truncated to 6 digits.
func TestTOTPReferenceValues(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	for _, c := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		if got := CodeAt(secret, time.Unix(c.unix, 0)); got != c.want {
			t.Fatalf("t=%d: %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestTOTPMatchWindowAndReplay(t *testing.T) {
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	cur := totpStep(now)
	for d := int64(-1); d <= 1; d++ {
		code, _ := totpCode(secret, cur+d)
		if step, ok := totpMatch(secret, code, now, 0); !ok || step != cur+d {
			t.Fatalf("skew %d: %v %d", d, ok, step)
		}
		// Not after a later accepted step.
		if _, ok := totpMatch(secret, code, now, cur+d); ok {
			t.Fatalf("skew %d replayed", d)
		}
	}
	far, _ := totpCode(secret, cur+2)
	if _, ok := totpMatch(secret, far, now, 0); ok {
		t.Fatal("a code two steps ahead was accepted")
	}
	if _, ok := totpMatch(secret, "12345", now, 0); ok {
		t.Fatal("a short code was accepted")
	}
}

func TestBackupCodes(t *testing.T) {
	c, err := newBackupCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 11 || c[5] != '-' {
		t.Fatalf("format: %q", c)
	}
	if string(hashBackupCode(c)) != string(hashBackupCode(" "+c[:5]+" "+c[6:])) {
		t.Fatal("spacing must not matter")
	}
	if isTOTPCode(c) || !isTOTPCode("012345") || isTOTPCode("01234a") {
		t.Fatal("isTOTPCode")
	}
}
