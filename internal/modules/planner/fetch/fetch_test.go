package fetch

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/modules/planner"
)

// A host name that resolves to a private address is refused at dial time,
// and the error shown to the owner never contains the URL.
func TestSafeRefusesPrivateAddresses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = Safe{}.Fetch(ctx, "https://localhost:"+port+"/secret-token.ics")
	if !errors.Is(err, planner.ErrCalendarURL) {
		t.Fatalf("expected a refusal, got %v", err)
	}
	if msg := planner.FetchError(err); strings.Contains(msg, "secret-token") {
		t.Fatalf("fetch error leaks the URL: %q", msg)
	}
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.45", "100.85.111.88", "169.254.169.254", "::1", "fe80::1", "0.0.0.0"} {
		if planner.PublicIP(net.ParseIP(ip)) {
			t.Errorf("%s counted as public", ip)
		}
	}
	if !planner.PublicIP(net.ParseIP("142.250.74.110")) {
		t.Error("a public address counted as private")
	}
}
