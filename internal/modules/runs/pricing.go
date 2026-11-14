// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package runs

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/ids"
)

// Billing of a run: subscription clients cost nothing per run, so their cost
// means "what this would cost on the API"; API-key agents pay it.
const (
	BillingSubscription = "subscription"
	BillingAPI          = "api"
)

// MaxPriceModelLen bounds a price's model pattern.
const MaxPriceModelLen = 100

// Errors of the price table and the currency setting.
var (
	ErrInvalidPrice = errors.New("a price needs a model (an exact name or a prefix ending in *) and non-negative amounts")
	ErrInvalidRate  = errors.New("the USD to HUF rate must be a positive number")
)

// Price is one row of the append-only price table, in USD per million tokens.
type Price struct {
	ID                ids.ID
	WorkspaceID       ids.ID
	Model             string // an exact model name, or a prefix ending in *
	InputPerMTok      float64
	OutputPerMTok     float64
	CacheReadPerMTok  float64
	CacheWritePerMTok float64
	ValidFrom         time.Time
	CreatedBy         ids.ID
	CreatedAt         time.Time
}

// Currency is the workspace's manual exchange rate.
type Currency struct {
	WorkspaceID ids.ID
	USDHUF      float64
	UpdatedBy   ids.ID
	UpdatedAt   time.Time
}

// Validate checks a price before it is stored.
func (p Price) Validate() error {
	m := p.Model
	if m == "" || m == "*" || utf8.RuneCountInString(m) > MaxPriceModelLen || strings.Contains(strings.TrimSuffix(m, "*"), "*") {
		return ErrInvalidPrice
	}
	for _, v := range []float64{p.InputPerMTok, p.OutputPerMTok, p.CacheReadPerMTok, p.CacheWritePerMTok} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrInvalidPrice
		}
	}
	return nil
}

// matches reports how well the pattern fits the model: its length, and
// whether it is exact; 0 means no match.
func (p Price) matches(model string) (length int, exact bool) {
	if prefix, ok := strings.CutSuffix(p.Model, "*"); ok {
		if strings.HasPrefix(model, prefix) {
			return len(prefix), false
		}
		return 0, false
	}
	if p.Model == model {
		return len(p.Model), true
	}
	return 0, false
}

// FindPrice returns the price of a model at a time: the longest pattern valid
// then (an exact name beats a prefix of the same length), and of a pattern
// its latest row. False when no price fits.
func FindPrice(prices []Price, model string, at time.Time) (Price, bool) {
	var best Price
	bestLen, bestExact, found := 0, false, false
	for _, p := range prices {
		if p.ValidFrom.After(at) {
			continue
		}
		length, exact := p.matches(model)
		if length == 0 {
			continue
		}
		better := !found || length > bestLen || (length == bestLen && exact && !bestExact) ||
			(length == bestLen && exact == bestExact && p.ValidFrom.After(best.ValidFrom))
		if better {
			best, bestLen, bestExact, found = p, length, exact, true
		}
	}
	return best, found
}

// EstimateUSD is the cost of the four token counts at the price valid at the
// run's time; nil when no price fits.
func EstimateUSD(prices []Price, model string, at time.Time, input, output, cacheRead, cacheWrite int64) *float64 {
	p, ok := FindPrice(prices, model, at)
	if !ok {
		return nil
	}
	usd := (float64(input)*p.InputPerMTok + float64(output)*p.OutputPerMTok +
		float64(cacheRead)*p.CacheReadPerMTok + float64(cacheWrite)*p.CacheWritePerMTok) / 1_000_000
	return &usd
}

// BillingOf tells whether a client runs under a subscription or on an API key.
func BillingOf(client string) string {
	switch client {
	case "claude-code", "antigravity":
		return BillingSubscription
	}
	return BillingAPI
}

// Events of the price table and the currency setting.
const (
	EventPriceAdded      = "settings.price_added"
	EventCurrencyChanged = "settings.currency_changed"
)

// PriceAdded is the payload of EventPriceAdded.
type PriceAdded struct {
	Model             string    `json:"model"`
	InputPerMTok      float64   `json:"input_per_mtok"`
	OutputPerMTok     float64   `json:"output_per_mtok"`
	CacheReadPerMTok  float64   `json:"cache_read_per_mtok"`
	CacheWritePerMTok float64   `json:"cache_write_per_mtok"`
	ValidFrom         time.Time `json:"valid_from"`
}

// CurrencyChanged is the payload of EventCurrencyChanged.
type CurrencyChanged struct {
	USDHUF float64 `json:"usd_huf"`
}
