// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"testing"
	"time"
)

func TestTimeoutMessage(t *testing.T) {
	for _, tc := range []struct {
		kind         runTimeoutType
		idle, maxRun time.Duration
		want         string
	}{
		{timeoutIdle, 59 * time.Second, time.Hour, "⏱ Leállítottam: 59 másodperce nem jött semmi a klienstől."},
		{timeoutIdle, time.Minute, time.Hour, "⏱ Leállítottam: 1 perce nem jött semmi a klienstől."},
		{timeoutIdle, 15*time.Minute + 30*time.Second, time.Hour, "⏱ Leállítottam: 15 perce nem jött semmi a klienstől."},
		{timeoutMaxRun, time.Minute, 4 * time.Hour, "⏱ Leállítottam: elérte a(z) 4 órás korlátot."},
		{timeoutMaxRun, time.Minute, time.Hour, "⏱ Leállítottam: elérte a(z) 1 órás korlátot."},
		{timeoutMaxRun, time.Minute, 59 * time.Minute, "⏱ Leállítottam: elérte a(z) 59 perces korlátot."},
		{timeoutMaxRun, time.Minute, time.Minute, "⏱ Leállítottam: elérte a(z) 1 perces korlátot."},
		{timeoutMaxRun, time.Minute, 59 * time.Second, "⏱ Leállítottam: elérte a(z) 59 másodperces korlátot."},
	} {
		if got := timeoutMessage(tc.kind, tc.idle, tc.maxRun); got != tc.want {
			t.Errorf("timeoutMessage(%s, %v, %v) = %q, want %q", tc.kind, tc.idle, tc.maxRun, got, tc.want)
		}
	}
}
