// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package clientip

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func mustList(t *testing.T, s string) []netip.Prefix {
	t.Helper()
	l, err := ParseList(s)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestParseList(t *testing.T) {
	l := mustList(t, " 192.0.2.7, 10.0.0.0/8 ,::1,, ::ffff:198.51.100.0/120 ")
	want := []string{"192.0.2.7/32", "10.0.0.0/8", "::1/128", "198.51.100.0/24"}
	if len(l) != len(want) {
		t.Fatalf("got %v", l)
	}
	for i, p := range l {
		if p.String() != want[i] {
			t.Errorf("%d: got %s, want %s", i, p, want[i])
		}
	}
	if l, err := ParseList(""); err != nil || len(l) != 0 {
		t.Fatalf("empty: %v %v", l, err)
	}
	for _, bad := range []string{"example.com", "10.0.0.0/33", "192.0.2.300", "fe80::1%eth0"} {
		if _, err := ParseList(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func req(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = remote
	for _, h := range xff {
		r.Header.Add("X-Forwarded-For", h)
	}
	return r
}

func TestForgedForwardedForIsIgnored(t *testing.T) {
	// No trusted proxies (the default): the header never changes the address.
	var res Resolver
	if got := res.Of(req("203.0.113.9:4444", "127.0.0.1")); got.String() != "203.0.113.9" {
		t.Fatalf("a forged X-Forwarded-For changed the client IP: %s", got)
	}
	// A trusted proxy is configured, but this request did not come from it.
	res = Resolver{Trusted: mustList(t, "10.0.0.2")}
	if got := res.Of(req("203.0.113.9:4444", "127.0.0.1")); got.String() != "203.0.113.9" {
		t.Fatalf("X-Forwarded-For from an untrusted peer was believed: %s", got)
	}
}

func TestTrustedProxyChain(t *testing.T) {
	res := Resolver{Trusted: mustList(t, "10.0.0.0/24")}
	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"one proxy", req("10.0.0.2:80", "198.51.100.4"), "198.51.100.4"},
		// The client wrote 127.0.0.1 itself; the proxy appended the real peer.
		{"forged left part", req("10.0.0.2:80", "127.0.0.1, 198.51.100.4"), "198.51.100.4"},
		{"two trusted hops", req("10.0.0.2:80", "198.51.100.4, 10.0.0.3"), "198.51.100.4"},
		{"several headers", req("10.0.0.2:80", "127.0.0.1", "198.51.100.4"), "198.51.100.4"},
		{"no header", req("10.0.0.2:80"), "10.0.0.2"},
		{"garbage hop", req("10.0.0.2:80", "1.2.3.4, nonsense"), "10.0.0.2"},
		{"all trusted", req("10.0.0.2:80", "10.0.0.5"), "10.0.0.5"},
		{"mapped v4", req("[::ffff:10.0.0.2]:80", "::ffff:198.51.100.4"), "198.51.100.4"},
	}
	for _, c := range cases {
		if got := res.Of(c.r); got.String() != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestPeerWithoutAddress(t *testing.T) {
	if got := (Resolver{}).Of(req("pipe")); got.IsValid() {
		t.Fatalf("got %s", got)
	}
	if String(netip.Addr{}) != "" {
		t.Fatal("zero address must print empty")
	}
}

func TestContains(t *testing.T) {
	l := mustList(t, "127.0.0.1/32,::1/128,100.64.0.0/10")
	for _, s := range []string{"127.0.0.1", "::1", "100.64.0.7", "::ffff:127.0.0.1"} {
		if !Contains(l, netip.MustParseAddr(s)) {
			t.Errorf("%s not contained", s)
		}
	}
	for _, s := range []string{"127.0.0.2", "192.0.2.1"} {
		if Contains(l, netip.MustParseAddr(s)) {
			t.Errorf("%s contained", s)
		}
	}
	if Contains(l, netip.Addr{}) {
		t.Error("the zero address is never contained")
	}
}

func TestHandlerStoresAddress(t *testing.T) {
	var got netip.Addr
	h := Resolver{}.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = From(r.Context()) }))
	h.ServeHTTP(httptest.NewRecorder(), req("192.0.2.1:1234"))
	if got.String() != "192.0.2.1" {
		t.Fatalf("got %s", got)
	}
}

func FuzzResolver(f *testing.F) {
	f.Add("10.0.0.2:80", "127.0.0.1, 198.51.100.4")
	f.Add("[::1]:1", "")
	f.Add("garbage", ",,,")
	res := Resolver{Trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}}
	f.Fuzz(func(t *testing.T, remote, xff string) {
		got := res.Of(req(remote, xff))
		peer := peerAddr(remote)
		// Without a trusted peer the header never matters.
		if !Contains(res.Trusted, peer) && got != peer {
			t.Fatalf("untrusted peer %q: header changed the address to %s", remote, got)
		}
		if got.IsValid() && got.Is4In6() {
			t.Fatalf("not unmapped: %s", got)
		}
	})
}
