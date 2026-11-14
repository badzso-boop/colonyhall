// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"testing"
	"time"
)

// The last line (the client's result event) must never be lost when the
// process exits right after writing it, even after a large output.
func TestExecStreamRunnerKeepsLastLine(t *testing.T) {
	script := `echo first; sleep 0.1; echo '{"type":"result"}'`
	for i := 0; i < 5; i++ {
		var last string
		slow := 0
		err := ExecStreamRunner(context.Background(), "", "sh", []string{"-c", script}, nil, func(line string) {
			if slow == 0 {
				slow++
				time.Sleep(300 * time.Millisecond) // a slow consumer, like a step callback doing I/O
			}
			last = line
		})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if last != `{"type":"result"}` {
			t.Fatalf("run %d: last line lost, got %q", i, clip(last, 40))
		}
	}
}
