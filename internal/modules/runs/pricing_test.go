package runs

import (
	"math"
	"testing"
	"time"
)

func TestEstimateUSD(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jun := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	prices := []Price{
		{Model: "claude-*", InputPerMTok: 1, OutputPerMTok: 2, ValidFrom: jan},
		{Model: "claude-opus-*", InputPerMTok: 10, OutputPerMTok: 20, CacheReadPerMTok: 1, CacheWritePerMTok: 5, ValidFrom: jan},
		{Model: "claude-opus-*", InputPerMTok: 15, OutputPerMTok: 30, CacheReadPerMTok: 1, CacheWritePerMTok: 5, ValidFrom: jun},
		{Model: "claude-opus-5-5", InputPerMTok: 100, OutputPerMTok: 200, ValidFrom: jun},
	}
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		model string
		at    time.Time
		in    int64
		out   int64
		cr    int64
		cw    int64
		want  float64
		none  bool
	}{
		{name: "no price", model: "gemini-3", at: mar, in: 1000, none: true},
		{name: "before any price", model: "claude-sonnet", at: jan.Add(-time.Hour), in: 1000, none: true},
		{name: "short prefix", model: "claude-sonnet-5", at: mar, in: 1_000_000, out: 1_000_000, want: 3},
		{name: "longest prefix wins", model: "claude-opus-4", at: mar, in: 1_000_000, want: 10},
		{name: "a later price keeps the old run's price", model: "claude-opus-4", at: mar, out: 1_000_000, want: 20},
		{name: "the new price after the change", model: "claude-opus-4", at: jul, out: 1_000_000, want: 30},
		{name: "exact beats prefix", model: "claude-opus-5-5", at: jul, in: 1_000_000, want: 100},
		{name: "exact not yet valid falls back", model: "claude-opus-5-5", at: mar, in: 1_000_000, want: 10},
		{name: "cache tokens", model: "claude-opus-4", at: mar, cr: 2_000_000, cw: 1_000_000, want: 7},
		{name: "fractions", model: "claude-opus-4", at: mar, in: 1500, want: 0.015},
	}
	for _, c := range cases {
		got := EstimateUSD(prices, c.model, c.at, c.in, c.out, c.cr, c.cw)
		if c.none {
			if got != nil {
				t.Errorf("%s: want no price, got %v", c.name, *got)
			}
			continue
		}
		if got == nil || math.Abs(*got-c.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPriceValidate(t *testing.T) {
	for _, bad := range []Price{{Model: ""}, {Model: "*"}, {Model: "a*b"}, {Model: "a*b*"}, {Model: "x", InputPerMTok: -1}, {Model: "x", OutputPerMTok: math.NaN()}, {Model: "x", CacheReadPerMTok: math.Inf(1)}} {
		if bad.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	for _, good := range []Price{{Model: "claude-*"}, {Model: "gemini-3.8-flash-low", InputPerMTok: 0.1}} {
		if err := good.Validate(); err != nil {
			t.Errorf("refused %+v: %v", good, err)
		}
	}
}

func TestBillingOf(t *testing.T) {
	for client, want := range map[string]string{"claude-code": BillingSubscription, "antigravity": BillingSubscription, "api": BillingAPI, "": BillingAPI} {
		if got := BillingOf(client); got != want {
			t.Errorf("%q: %s", client, got)
		}
	}
}
