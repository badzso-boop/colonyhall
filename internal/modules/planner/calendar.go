package planner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
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

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// PublicIP reports whether ip is a public unicast address (not loopback,
// private, link-local, CGNAT 100.64/10, 0.0.0.0/8 or multicast).
func PublicIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
		if ip4[0] == 0 || cgnat.Contains(ip4) {
			return false
		}
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.IsMulticast()
}

// fetchError turns a fetch failure into a message for the owner that never
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
	for _, ev := range masters {
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
		uid, _ := ev.Props.Text(ical.PropUID)
		for _, start := range set.Between(from.Add(-dur), to, true) {
			if overridden[key{uid, start.Unix()}] {
				continue
			}
			e := base
			e.Start, e.End = start, start.Add(dur)
			if e.End.After(from) && e.Start.Before(to) {
				out = append(out, e)
			}
			if len(out) >= maxOccurrences {
				break
			}
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
