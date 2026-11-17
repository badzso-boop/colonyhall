// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package egress is the server's outbound HTTP for the Workshop (round 56,
// docs/25-onfejlesztes-terv.md 13.7): connectors and extension code reach
// public https addresses only. Every dial resolves the host and connects to
// a checked address (no DNS rebinding), every redirect is checked again,
// and bodies and time are capped.
//
// On a self-hosted server the operator may allow internal addresses
// (COLONY_EGRESS_ALLOW: CIDRs and host names, e.g. the team's Gitea on the
// LAN); a multi-tenant server refuses that setting at start.
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Errors.
var (
	// ErrAddress: the URL points to an address the policy refuses.
	ErrAddress = errors.New("egress: the address is not allowed (only public https addresses)")
	// ErrScheme: not https (plain http only to an allowed internal host).
	ErrScheme = errors.New("egress: only https is allowed")
)

// MaxRedirects bounds the redirects followed.
const MaxRedirects = 5

// nonPublic lists the IANA special-purpose ranges (the same list as the
// planner's calendar fetcher). NAT64, 6to4 and Teredo are refused whole:
// they can embed any IPv4 address.
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

// Public reports whether addr is a public unicast address.
func Public(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// Policy says which addresses beyond the public internet are allowed.
// The zero value allows public https only.
type Policy struct {
	// Prefixes are internal networks the operator allowed.
	Prefixes []netip.Prefix
	// Hosts are internal host names the operator allowed (lower case);
	// they may resolve to any address.
	Hosts []string
}

// Empty reports whether nothing beyond the public internet is allowed.
func (p Policy) Empty() bool { return len(p.Prefixes) == 0 && len(p.Hosts) == 0 }

// ParsePolicy reads COLONY_EGRESS_ALLOW: CIDRs, addresses and host names,
// separated by commas.
func ParsePolicy(s string) (Policy, error) {
	var p Policy
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(strings.ToLower(f))
		if f == "" {
			continue
		}
		if pr, err := netip.ParsePrefix(f); err == nil {
			p.Prefixes = append(p.Prefixes, pr.Masked())
			continue
		}
		if a, err := netip.ParseAddr(f); err == nil {
			p.Prefixes = append(p.Prefixes, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
			continue
		}
		if strings.ContainsAny(f, "/: ") || strings.HasPrefix(f, ".") || strings.HasSuffix(f, ".") {
			return Policy{}, fmt.Errorf("egress: %q is neither a CIDR, an address nor a host name", f)
		}
		p.Hosts = append(p.Hosts, f)
	}
	return p, nil
}

func (p Policy) hostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range p.Hosts {
		if h == host {
			return true
		}
	}
	return false
}

func (p Policy) addrAllowed(addr netip.Addr) bool {
	addr = addr.Unmap()
	if Public(addr) {
		return true
	}
	for _, pr := range p.Prefixes {
		if pr.Contains(addr) {
			return true
		}
	}
	return false
}

// internalTarget reports whether host is an allowed internal target (an
// allowed host name, or a literal address inside an allowed prefix).
func (p Policy) internalTarget(host string) bool {
	if p.hostAllowed(host) {
		return true
	}
	if a, err := netip.ParseAddr(host); err == nil {
		for _, pr := range p.Prefixes {
			if pr.Contains(a.Unmap()) {
				return true
			}
		}
	}
	return false
}

// CheckURL validates a URL before any request: https (plain http only to
// an allowed internal target), a host, no user info. Allowed prefixes make
// http allowed for literal addresses in them, but a name is checked at dial
// time only.
func (p Policy) CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("egress: invalid URL")
	}
	if u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("egress: the URL needs a host and no user name")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !p.internalTarget(u.Hostname()) {
			return nil, ErrScheme
		}
	default:
		return nil, ErrScheme
	}
	return u, nil
}

// Options tune a client.
type Options struct {
	Timeout time.Duration // whole request (default 20 s)
	// Hosts, when not empty, limits the requests to these host names
	// (an extension's declared egress, round 56b).
	Hosts []string
	// Resolver replaces the system resolver (tests).
	Resolver interface {
		LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
	}
}

// Client returns an HTTP client that only reaches what the policy allows.
func (p Policy) Client(o Options) *http.Client {
	if o.Timeout <= 0 {
		o.Timeout = 20 * time.Second
	}
	var resolver = o.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			var addrs []netip.Addr
			if a, err := netip.ParseAddr(host); err == nil {
				addrs = []netip.Addr{a}
			} else {
				addrs, err = resolver.LookupNetIP(ctx, "ip", host)
				if err != nil {
					return nil, err
				}
			}
			named := p.hostAllowed(host)
			for _, a := range addrs {
				if named || p.addrAllowed(a) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
				}
			}
			return nil, ErrAddress
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: o.Timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	hosts := o.Hosts
	return &http.Client{
		Transport: &checking{policy: p, hosts: hosts, next: tr},
		Timeout:   o.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > MaxRedirects {
				return errors.New("egress: too many redirects")
			}
			return nil // the transport checks the new URL
		},
	}
}

// checking validates every request's URL (redirects included) before the
// dial.
type checking struct {
	policy Policy
	hosts  []string
	next   http.RoundTripper
}

func (c *checking) RoundTrip(r *http.Request) (*http.Response, error) {
	if _, err := c.policy.CheckURL(r.URL.String()); err != nil {
		return nil, err
	}
	if len(c.hosts) > 0 && !hostIn(r.URL.Hostname(), c.hosts) {
		return nil, fmt.Errorf("egress: %s is not among the declared hosts", r.URL.Hostname())
	}
	return c.next.RoundTrip(r)
}

func hostIn(h string, list []string) bool {
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	for _, x := range list {
		if strings.ToLower(x) == h {
			return true
		}
	}
	return false
}
