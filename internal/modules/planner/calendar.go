package planner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Fetch limits for a calendar subscription.
const (
	MaxCalendarBytes = 5 << 20
	FetchTimeout     = 20 * time.Second
	MaxRedirects     = 3
	maxDescription   = 1000
	maxOccurrences   = 2000
	maxRuleSteps     = 100000  // rule iterations per recurring event
	maxTotalSteps    = 1000000 // rule iterations per calendar, all events together
	maxEventLength   = 31 * 24 * time.Hour
)

// Fetcher downloads a calendar (the fetch package has the safe one).
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) ([]byte, error)
}

// NormalizeCalendarURL maps webcal:// to https:// and accepts only https
// URLs with a host name or a public IP. Private addresses are refused again
// at dial time, after DNS resolution.
func NormalizeCalendarURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "webcal://") {
		raw = "https://" + raw[len("webcal://"):]
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", ErrCalendarURL
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !PublicIP(ip) {
		return "", ErrCalendarURL
	}
	return u.String(), nil
}

// MaskURL shows the host and the last 4 characters: the URL carries a token.
func MaskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "…"
	}
	tail := raw
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return u.Host + "/…" + tail
}

// nonPublic lists the IANA special-purpose ranges a calendar host must not
// resolve to. NAT64, 6to4 and Teredo are refused whole: they can embed any
// IPv4 address, and no calendar provider needs them.
var nonPublic = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32",
		"2002::/16", "fc00::/7", "fe80::/10", "fec0::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// PublicIP reports whether ip is a public unicast address: not in any of
// the special-purpose ranges above (IPv4-mapped IPv6 is checked as IPv4).
func PublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, p := range nonPublic {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// FetchError turns a fetch failure into a message for the owner that never
// contains the URL (it carries a token).
func FetchError(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	switch {
	case errors.Is(err, ErrCalendarURL):
		return "the calendar host is not a public address"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	}
	msg := err.Error()
	if strings.Contains(msg, "://") {
		return "download failed"
	}
	return msg
}

// ParseCalendar returns the occurrences of an ICS document that overlap
// [from, to): recurrences expanded (RRULE, RDATE, EXDATE), moved instances
// (RECURRENCE-ID) replacing the original one, cancelled events dropped.
// Floating times and unknown time zones (e.g. Outlook's Windows names) are
// read in loc.
func ParseCalendar(data []byte, loc *time.Location, from, to time.Time) ([]Event, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("not a calendar: %w", err)
	}
	type key struct {
		uid string
		at  int64
	}
	var out []Event
	overridden := map[key]bool{}
	var masters []ical.Event
	for _, ev := range cal.Events() {
		dropUnknownZones(ev.Component)
		if p := ev.Props.Get(ical.PropRecurrenceID); p != nil {
			uid, _ := ev.Props.Text(ical.PropUID)
			if t, err := p.DateTime(loc); err == nil {
				overridden[key{uid, t.Unix()}] = true
			}
			if e, ok := single(ev, loc, from, to); ok {
				out = append(out, e)
			}
			continue
		}
		masters = append(masters, ev)
	}
	budget := maxTotalSteps
	for _, ev := range masters {
		if budget <= 0 {
			break
		}
		if cancelled(ev) {
			continue
		}
		set, err := ev.RecurrenceSet(loc)
		if err != nil {
			continue // a broken rule must not hide the rest of the calendar
		}
		if set == nil {
			if e, ok := single(ev, loc, from, to); ok {
				out = append(out, e)
			}
			continue
		}
		base, ok := fields(ev, loc)
		if !ok {
			continue
		}
		dur := base.End.Sub(base.Start)
		if dur > maxEventLength {
			dur = maxEventLength
		}
		uid, _ := ev.Props.Text(ical.PropUID)
		// Walk the rule with an iterator and a step budget, so a hostile
		// rule (FREQ=SECONDLY, a start decades ago) cannot allocate or spin
		// without bound; Between would build the whole list first.
		next := set.Iterator()
		for steps := 0; steps < maxRuleSteps && budget > 0 && len(out) < maxOccurrences; steps++ {
			budget--
			start, ok := next()
			if !ok || !start.Before(to) {
				break
			}
			if !start.Add(dur).After(from) || overridden[key{uid, start.Unix()}] {
				continue
			}
			e := base
			e.Start, e.End = start, start.Add(dur)
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	if len(out) > maxOccurrences {
		out = out[:maxOccurrences]
	}
	return out, nil
}

func cancelled(ev ical.Event) bool {
	st, _ := ev.Props.Text(ical.PropStatus)
	return strings.EqualFold(st, "CANCELLED")
}

func single(ev ical.Event, loc *time.Location, from, to time.Time) (Event, bool) {
	if cancelled(ev) {
		return Event{}, false
	}
	e, ok := fields(ev, loc)
	if !ok || !e.End.After(from) || !e.Start.Before(to) {
		return Event{}, false
	}
	return e, true
}

func fields(ev ical.Event, loc *time.Location) (Event, bool) {
	start, err := ev.DateTimeStart(loc)
	if err != nil || start.IsZero() {
		return Event{}, false
	}
	end, err := ev.DateTimeEnd(loc)
	if err != nil || !end.After(start) {
		end = start
	}
	title, _ := ev.Props.Text(ical.PropSummary)
	where, _ := ev.Props.Text(ical.PropLocation)
	desc, _ := ev.Props.Text(ical.PropDescription)
	if r := []rune(desc); len(r) > maxDescription {
		desc = string(r[:maxDescription]) + "…"
	}
	allDay := false
	if p := ev.Props.Get(ical.PropDateTimeStart); p != nil {
		allDay = p.ValueType() == ical.ValueDate || (p.ValueType() == ical.ValueDefault && len(p.Value) == 8)
	}
	return Event{Title: strings.TrimSpace(title), Start: start, End: end, AllDay: allDay,
		Location: strings.TrimSpace(where), Description: strings.TrimSpace(desc)}, true
}

// dropUnknownZones removes TZID parameters Go cannot load, so the time is
// read in the default location instead of failing the event.
func dropUnknownZones(comp *ical.Component) {
	for name, props := range comp.Props {
		for i := range props {
			if tz := props[i].Params.Get(ical.PropTimezoneID); tz != "" {
				if _, err := time.LoadLocation(tz); err != nil {
					props[i].Params.Del(ical.PropTimezoneID)
				}
			}
		}
		comp.Props[name] = props
	}
}
