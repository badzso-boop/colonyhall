// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package egress

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return f[host], nil
}

func TestPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"1.1.1.1": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "192.168.0.50": false, "100.64.0.9": false,
		"169.254.169.254": false, "::1": false, "::ffff:127.0.0.1": false, "fd00::1": false,
	} {
		if got := Public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Public(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestParsePolicy(t *testing.T) {
	p, err := ParsePolicy(" 192.168.1.0/24, 100.64.0.7 ,gitea.lan ")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Prefixes) != 2 || len(p.Hosts) != 1 || p.Hosts[0] != "gitea.lan" {
		t.Fatalf("policy = %+v", p)
	}
	if !p.addrAllowed(netip.MustParseAddr("192.168.1.9")) || p.addrAllowed(netip.MustParseAddr("192.168.2.9")) {
		t.Fatal("prefix check")
	}
	for _, bad := range []string{"http://x", "a b", ".lan"} {
		if _, err := ParsePolicy(bad); err == nil {
			t.Errorf("ParsePolicy(%q) accepted", bad)
		}
	}
	if q, _ := ParsePolicy(""); !q.Empty() {
		t.Fatal("empty policy")
	}
}

func TestCheckURL(t *testing.T) {
	var p Policy
	for raw, ok := range map[string]bool{
		"https://api.example.com/x":   true,
		"http://api.example.com/x":    false, // plain http only to an allowed internal target
		"ftp://example.com":           false,
		"https://user:pw@example.com": false,
		"https:///nohost":             false,
	} {
		if _, err := p.CheckURL(raw); (err == nil) != ok {
			t.Errorf("CheckURL(%q) err = %v", raw, err)
		}
	}
	lan, _ := ParsePolicy("gitea.lan,192.168.1.0/24")
	if _, err := lan.CheckURL("http://gitea.lan:3000/api"); err != nil {
		t.Errorf("allowed internal host over http: %v", err)
	}
	if _, err := lan.CheckURL("http://192.168.1.5/api"); err != nil {
		t.Errorf("allowed internal address over http: %v", err)
	}
}

func serverURL(t *testing.T, h http.HandlerFunc) *url.URL {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u
}

// A loopback server is refused unless the operator allowed it.
func TestClientRefusesInternal(t *testing.T) {
	u := serverURL(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	var p Policy
	if res, err := p.Client(Options{}).Get(u.String()); err == nil {
		_ = res.Body.Close()
		t.Fatal("loopback reached without an allow rule")
	}

	allowed, _ := ParsePolicy("127.0.0.1")
	res, err := allowed.Client(Options{}).Get(u.String())
	if err != nil {
		t.Fatalf("allowed loopback: %v", err)
	}
	_ = res.Body.Close()
}

// DNS rebinding: a name that resolves to an internal address is refused at
// dial time even though the URL looked fine.
func TestClientRefusesNameResolvingInternal(t *testing.T) {
	var p Policy
	c := p.Client(Options{Resolver: fakeResolver{"evil.example.com": {netip.MustParseAddr("10.0.0.5")}}})
	res, err := c.Get("https://evil.example.com/")
	if err == nil {
		_ = res.Body.Close()
	}
	if !errors.Is(err, ErrAddress) {
		t.Fatalf("err = %v, want ErrAddress", err)
	}
}

// A redirect to an internal address is checked again.
func TestClientChecksRedirects(t *testing.T) {
	target := serverURL(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secret")) })
	first := serverURL(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.String(), http.StatusFound)
	})
	// Only the name first.test is allowed; the redirect goes to a literal
	// loopback address without an allow rule and must fail.
	q, _ := ParsePolicy("first.test")
	c := q.Client(Options{Resolver: fakeResolver{"first.test": {netip.MustParseAddr(first.Hostname())}}})
	if res, err := c.Get("http://first.test:" + first.Port() + "/"); err == nil {
		_ = res.Body.Close()
		t.Fatal("redirect to a refused address was followed")
	}
}

// Declared hosts limit an extension's requests.
func TestClientDeclaredHosts(t *testing.T) {
	q, _ := ParsePolicy("a.test,b.test")
	u := serverURL(t, func(w http.ResponseWriter, _ *http.Request) {})
	res := fakeResolver{"a.test": {netip.MustParseAddr(u.Hostname())}, "b.test": {netip.MustParseAddr(u.Hostname())}}
	c := q.Client(Options{Hosts: []string{"a.test"}, Resolver: res})
	if r, err := c.Get("http://a.test:" + u.Port()); err != nil {
		t.Fatalf("declared host: %v", err)
	} else {
		_ = r.Body.Close()
	}
	if r, err := c.Get("http://b.test:" + u.Port()); err == nil {
		_ = r.Body.Close()
		t.Fatal("undeclared host reached")
	}
}

// The first address may be unreachable: the next allowed one is tried.
func TestClientTriesTheNextAddress(t *testing.T) {
	u := serverURL(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	q, _ := ParsePolicy("two.test")
	// 127.0.0.2 has nothing listening on the port; 127.0.0.1 has the server.
	c := q.Client(Options{Resolver: fakeResolver{"two.test": {netip.MustParseAddr("127.0.0.2"), netip.MustParseAddr(u.Hostname())}}})
	res, err := c.Get("http://two.test:" + u.Port())
	if err != nil {
		t.Fatalf("the second address was not tried: %v", err)
	}
	_ = res.Body.Close()
}
