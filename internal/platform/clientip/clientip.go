// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package clientip decides which address a request comes from (round 40).
//
// The answer is the TCP peer (RemoteAddr). X-Forwarded-For is believed only
// when the peer is a configured trusted proxy (COLONY_TRUSTED_PROXIES), and
// then only up to the first address that is not itself a trusted proxy:
// everything left of it was written by the client and can be forged.
package clientip

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseList reads a comma-separated list of IP addresses and CIDR
// prefixes ("192.0.2.7, 10.0.0.0/8, ::1"). A bare address is a single-host
// prefix. Empty input is an empty list.
func ParseList(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			p, err := netip.ParsePrefix(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not an IP address or CIDR prefix", part)
			}
			out = append(out, unmapPrefix(p).Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil || a.Zone() != "" {
			return nil, fmt.Errorf("%q is not an IP address or CIDR prefix", part)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func unmapPrefix(p netip.Prefix) netip.Prefix {
	if !p.Addr().Is4In6() {
		return p
	}
	bits := p.Bits() - 96
	if bits < 0 {
		bits = 0
	}
	return netip.PrefixFrom(p.Addr().Unmap(), bits)
}

// Contains reports whether a is in one of the prefixes.
func Contains(list []netip.Prefix, a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.Unmap()
	for _, p := range list {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Resolver finds the client address of a request.
type Resolver struct {
	// Trusted are the reverse proxies whose X-Forwarded-For is believed.
	// Empty (the default): the header is ignored.
	Trusted []netip.Prefix
}

// Of returns the client address of r; the zero Addr when RemoteAddr is
// not an address (an in-process test request, a Unix socket).
func (res Resolver) Of(r *http.Request) netip.Addr {
	peer := peerAddr(r.RemoteAddr)
	if !peer.IsValid() || !Contains(res.Trusted, peer) {
		return peer
	}
	// Walk the chain from the right: each hop appended the address it saw.
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break // a malformed hop: stop at the last address we trust
		}
		client = a.Unmap()
		if !Contains(res.Trusted, client) {
			break
		}
	}
	return client
}

func peerAddr(remote string) netip.Addr {
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.WithZone("").Unmap()
}

type key struct{}

// With returns ctx carrying the client address.
func With(ctx context.Context, a netip.Addr) context.Context {
	return context.WithValue(ctx, key{}, a)
}

// From returns the client address the middleware stored; the zero Addr
// when there is none.
func From(ctx context.Context) netip.Addr {
	a, _ := ctx.Value(key{}).(netip.Addr)
	return a
}

// Handler stores the client address of every request in its context.
func (res Resolver) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(With(r.Context(), res.Of(r))))
	})
}

// String is the address's canonical text form; "" for the zero Addr.
func String(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}
