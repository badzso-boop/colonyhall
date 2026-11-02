package runs

import (
	"context"
	"errors"
	"sort"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// MaxRunPage bounds a page of runs.
const MaxRunPage = 200

func adminOnly(by identity.Actor) error {
	if by.Kind != identity.KindHuman || !by.IsAdmin() {
		return ErrForbidden
	}
	return nil
}

// SearchRuns lists runs by filter for humans, with their estimated cost at
// the price of their time. next is set when there may be more.
func (s *Service) SearchRuns(ctx context.Context, by identity.Actor, f RunFilter) (list []Run, next *RunCursor, err error) {
	if by.Kind != identity.KindHuman {
		return nil, nil, ErrForbidden
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	f.Limit = min(f.Limit, MaxRunPage)
	limit := f.Limit
	f.Limit = limit + 1
	list, err = s.Store.SearchRuns(ctx, by.WorkspaceID, f)
	if err != nil {
		return nil, nil, err
	}
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1]
		next = &RunCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	prices, err := s.Store.Prices(ctx, by.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	for i := range list {
		r := &list[i]
		r.Billing = BillingOf(r.Client)
		r.EstimatedUSD = EstimateUSD(prices, r.Model, r.CreatedAt, r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheWriteTokens)
	}
	return list, next, nil
}

// UsageSummary returns usage per agent and day, per group and day, and the
// totals of the last days, each run costed at the price of its time.
func (s *Service) UsageSummary(ctx context.Context, by identity.Actor, days int, groupBy string) (UsageSummary, error) {
	if by.Kind != identity.KindHuman {
		return UsageSummary{}, ErrForbidden
	}
	if days < 1 || days > 90 {
		return UsageSummary{}, ErrInvalidDays
	}
	if groupBy == "" {
		groupBy = GroupByAgent
	}
	if groupBy != GroupByAgent && groupBy != GroupByHost && groupBy != GroupByModel {
		return UsageSummary{}, ErrInvalidGroup
	}
	now := s.Clock.Now().In(Budapest)
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Budapest).AddDate(0, 0, -(days - 1))
	rows, err := s.Store.UsageRuns(ctx, by.WorkspaceID, since)
	if err != nil {
		return UsageSummary{}, err
	}
	prices, err := s.Store.Prices(ctx, by.WorkspaceID)
	if err != nil {
		return UsageSummary{}, err
	}
	return summarize(rows, prices, groupBy), nil
}

// summarize adds the runs up; a pure function so the rounding and the
// grouping are tested without a store.
func summarize(rows []UsageRun, prices []Price, groupBy string) UsageSummary {
	type dayKey struct {
		agent ids.ID
		day   string
	}
	type groupKey struct{ key, day string }
	days := map[dayKey]*DayUsage{}
	groups := map[groupKey]*UsageGroup{}
	var out UsageSummary
	var duration int64
	add := func(p **float64, usd *float64) {
		if usd == nil {
			return
		}
		if *p == nil {
			v := 0.0
			*p = &v
		}
		**p += *usd
	}
	for _, r := range rows {
		day := r.CreatedAt.In(Budapest).Format("2006-01-02")
		usd := EstimateUSD(prices, r.Model, r.CreatedAt, r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheWriteTokens)
		total := r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
		failed := 0
		if r.Status == StatusError {
			failed = 1
		}

		d := days[dayKey{r.AgentID, day}]
		if d == nil {
			d = &DayUsage{AgentID: r.AgentID, Day: day}
			days[dayKey{r.AgentID, day}] = d
		}
		d.InputTokens += r.InputTokens
		d.OutputTokens += r.OutputTokens
		d.CacheReadTokens += r.CacheReadTokens
		d.CacheWriteTokens += r.CacheWriteTokens
		d.TotalTokens += total
		d.RunCount++
		d.ErrorCount += failed
		add(&d.EstimatedUSD, usd)

		key := r.AgentID.String()
		switch groupBy {
		case GroupByHost:
			key = ""
			if r.HostID != ids.Nil {
				key = r.HostID.String()
			}
		case GroupByModel:
			key = r.Model
		}
		g := groups[groupKey{key, day}]
		if g == nil {
			g = &UsageGroup{Key: key, Day: day}
			groups[groupKey{key, day}] = g
		}
		g.InputTokens += r.InputTokens
		g.OutputTokens += r.OutputTokens
		g.CacheReadTokens += r.CacheReadTokens
		g.CacheWriteTokens += r.CacheWriteTokens
		g.TotalTokens += total
		g.RunCount++
		g.ErrorCount += failed
		add(&g.EstimatedUSD, usd)

		t := &out.Totals
		t.InputTokens += r.InputTokens
		t.OutputTokens += r.OutputTokens
		t.CacheReadTokens += r.CacheReadTokens
		t.CacheWriteTokens += r.CacheWriteTokens
		t.TotalTokens += total
		t.RunCount++
		t.ErrorCount += failed
		duration += int64(r.DurationMS)
		switch {
		case usd == nil:
			t.UnpricedRuns++
		case BillingOf(r.Client) == BillingSubscription:
			t.SubscriptionUSD += *usd
		default:
			t.APIUSD += *usd
		}
	}
	if out.Totals.RunCount > 0 {
		out.Totals.AvgDurationMS = int(duration / int64(out.Totals.RunCount))
	}
	out.Days = make([]DayUsage, 0, len(days))
	for _, d := range days {
		out.Days = append(out.Days, *d)
	}
	sort.Slice(out.Days, func(i, j int) bool {
		if out.Days[i].Day != out.Days[j].Day {
			return out.Days[i].Day > out.Days[j].Day
		}
		return out.Days[i].AgentID.String() < out.Days[j].AgentID.String()
	})
	out.Groups = make([]UsageGroup, 0, len(groups))
	for _, g := range groups {
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool {
		if out.Groups[i].Day != out.Groups[j].Day {
			return out.Groups[i].Day > out.Groups[j].Day
		}
		return out.Groups[i].Key < out.Groups[j].Key
	})
	return out
}

// Prices lists the price table for admins, history included.
func (s *Service) Prices(ctx context.Context, by identity.Actor) ([]Price, error) {
	if err := adminOnly(by); err != nil {
		return nil, err
	}
	return s.Store.Prices(ctx, by.WorkspaceID)
}

// AddPrice adds a price row (admins, audited). It never changes an old row,
// so old runs keep the price of their time.
func (s *Service) AddPrice(ctx context.Context, by identity.Actor, p Price) (Price, error) {
	if err := adminOnly(by); err != nil {
		return Price{}, err
	}
	if err := p.Validate(); err != nil {
		return Price{}, err
	}
	now := s.Clock.Now()
	p.ID, p.WorkspaceID, p.CreatedBy, p.CreatedAt = s.IDs.New(), by.WorkspaceID, by.ID, now
	if p.ValidFrom.IsZero() {
		p.ValidFrom = now
	}
	err := s.record(ctx, by, EventPriceAdded, p.ID, PriceAdded{Model: p.Model, InputPerMTok: p.InputPerMTok, OutputPerMTok: p.OutputPerMTok,
		CacheReadPerMTok: p.CacheReadPerMTok, CacheWritePerMTok: p.CacheWritePerMTok, ValidFrom: p.ValidFrom}, func(ctx context.Context) error {
		return s.Store.InsertPrice(ctx, p)
	})
	if err != nil {
		return Price{}, err
	}
	return p, nil
}

// Currency returns the exchange rate for admins; ok is false until it is set.
func (s *Service) Currency(ctx context.Context, by identity.Actor) (c Currency, ok bool, err error) {
	if err := adminOnly(by); err != nil {
		return Currency{}, false, err
	}
	c, err = s.Store.Currency(ctx, by.WorkspaceID)
	if errors.Is(err, db.ErrNotFound) {
		return Currency{}, false, nil
	}
	return c, err == nil, err
}

// SetCurrency sets the manual USD to HUF rate (admins, audited). Costs are
// stored in USD and HUF is computed in the client, so this rewrites nothing.
func (s *Service) SetCurrency(ctx context.Context, by identity.Actor, usdHuf float64) (Currency, error) {
	if err := adminOnly(by); err != nil {
		return Currency{}, err
	}
	if !(usdHuf > 0) || usdHuf > 1_000_000 {
		return Currency{}, ErrInvalidRate
	}
	c := Currency{WorkspaceID: by.WorkspaceID, USDHUF: usdHuf, UpdatedBy: by.ID, UpdatedAt: s.Clock.Now()}
	err := s.record(ctx, by, EventCurrencyChanged, by.WorkspaceID, CurrencyChanged{USDHUF: usdHuf}, func(ctx context.Context) error {
		return s.Store.PutCurrency(ctx, c)
	})
	if err != nil {
		return Currency{}, err
	}
	return c, nil
}

func (s *Service) record(ctx context.Context, by identity.Actor, typ string, target ids.ID, payload any, write func(context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := write(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, typ, by.ID, ids.Nil, "settings", target, payload, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}
