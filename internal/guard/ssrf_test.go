package guard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Round 40: the address check of web_read and the sandbox proxy.
func TestBlockedIPs(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "0.1.2.3",
		"198.18.0.1", "192.0.0.8", "240.0.0.1", "255.255.255.255", "224.0.0.1",
		"::1", "::", "fe80::1", "fd00:ec2::254", "ff02::1",
		"::ffff:10.0.0.1",     // IPv4-mapped
		"64:ff9b::a9fe:a9fe",  // NAT64 of 169.254.169.254
		"64:ff9b:1::a00:1",    // local-use NAT64
		"2002:a00:1::1",       // 6to4 of 10.0.0.1
		"2001:0:4136:e378::1", // Teredo
	}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111", "::ffff:93.184.216.34"} {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s is a public address", s)
		}
	}
	if !isBlockedIP(nil) {
		t.Error("an unparsable address is blocked")
	}
}

func TestHostPatternIgnoresTrailingDot(t *testing.T) {
	for _, host := range []string{"pastebin.com.", "PASTEBIN.com", "pastebin.com"} {
		if !matchHostPattern("pastebin.com", host) {
			t.Errorf("%q must match the deny pattern", host)
		}
		if !matchHostPattern("*.pastebin.com.", "x."+host) {
			t.Errorf("x.%q must match the wildcard", host)
		}
	}
}

func TestDeniedHostWithTrailingDotIsRefused(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("secret")) }))
	res.mapping["example.test"] = []net.IP{net.ParseIP("93.184.216.34")}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	_, err := Fetch(context.Background(), fmt.Sprintf("http://example.test.:%s/", u.Port()), FetchOptions{Deny: []string{"example.test"}, Resolver: res, DialContext: dial})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("a denied host with a trailing dot was fetched: %v", err)
	}
}

func TestBlockedAddressIsNotNamedToTheAgent(t *testing.T) {
	_, res := setupTestServer(t, http.NotFoundHandler())
	_, err := Fetch(context.Background(), "http://private.test/", FetchOptions{Resolver: res})
	if err == nil || strings.Contains(err.Error(), "192.168.1.1") {
		t.Fatalf("the private address leaked to the agent: %v", err)
	}
}
