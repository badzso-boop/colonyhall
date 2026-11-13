package decision

import (
	"math"
	"testing"
)

// Round 40: an agent's decision_list limit is bounded; one past int32 used
// to turn negative in SQL.
func TestClampLimit(t *testing.T) {
	for in, want := range map[int]int{math.MaxInt: MaxListLimit, math.MaxInt32 + 1: MaxListLimit, -5: 1, 0: 1, 7: 7, MaxListLimit: MaxListLimit} {
		if got := clampLimit(in); got != want {
			t.Errorf("clampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}
