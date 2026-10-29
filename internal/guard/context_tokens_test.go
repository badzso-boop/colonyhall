package guard

import "testing"

// The run report carries the session's context size, which the agent
// overview compares with the session limit; the summed usage of a run
// counts the same context once per tool call and must not stand in for it.
func TestContextTokensForRunReport(t *testing.T) {
	usage := Usage{Input: 100, Output: 900, CacheRead: 20_000, CacheWrite: 300}
	cases := []struct {
		name   string
		client string
		res    RunResult
		want   int
	}{
		{"claude measured", "claude-code", RunResult{Usage: usage, ContextTokens: 42_000}, 42_000},
		{"claude from usage", "claude-code", RunResult{Usage: usage}, 20_400},
		{"antigravity measured", "antigravity", RunResult{Usage: usage, ContextTokens: 7_000}, 7_000},
		{"antigravity unknown", "antigravity", RunResult{Usage: usage}, 0},
	}
	for _, c := range cases {
		a := &Agent{Cfg: AgentConfig{Client: c.client}}
		if got := a.contextTokens(c.res); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}
