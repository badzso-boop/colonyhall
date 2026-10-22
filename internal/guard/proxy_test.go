package guard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockResolver struct {
	mapping map[string][]net.IP
}

func (m *mockResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ips, ok := m.mapping[strings.ToLower(host)]; ok {
		var addrs []net.IPAddr
		for _, ip := range ips {
			addrs = append(addrs, net.IPAddr{IP: ip})
		}
		return addrs, nil
	}
	return nil, errors.New("host not found")
}

func startEchoServer(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port
	return l, port
}

func startHTTPServer(t *testing.T, respText string) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				r := bufio.NewReader(c)
				req, err := http.ReadRequest(r)
				if err != nil {
					return
				}
				resp := &http.Response{
					StatusCode: http.StatusOK,
					ProtoMajor: 1,
					ProtoMinor: 1,
					Body:       io.NopCloser(strings.NewReader(respText)),
					Header:     make(http.Header),
					Request:    req,
				}
				resp.Header.Set("Content-Type", "text/plain")
				_ = resp.Write(c)
			}(conn)
		}
	}()
	port := l.Addr().(*net.TCPAddr).Port
	return l, port
}

func dialProxyUnix(t *testing.T, sockPath string) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to dial proxy unix socket: %v", err)
	}
	return conn
}

func TestProxy_AllowedCONNECT(t *testing.T) {
	echoL, echoPort := startEchoServer(t)
	defer func() { _ = echoL.Close() }()

	sockPath := filepath.Join(t.TempDir(), "proxy.sock")
	res := &mockResolver{
		mapping: map[string][]net.IP{
			"api.anthropic.com": {net.ParseIP("93.184.216.34")},
		},
	}
	pr, err := StartSandboxProxy(ProxyConfig{
		Mode:     "filtered",
		Allow:    []string{fmt.Sprintf("api.anthropic.com:%d", echoPort)},
		SockPath: sockPath,
		Resolver: res,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("tcp", echoL.Addr().String())
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()

	// Connect through proxy
	conn := dialProxyUnix(t, sockPath)
	defer func() { _ = conn.Close() }()

	connectReq := fmt.Sprintf("CONNECT api.anthropic.com:%d HTTP/1.1\r\nHost: api.anthropic.com:%d\r\n\r\n", echoPort, echoPort)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		t.Fatal(err)
	}

	r := bufio.NewReader(conn)
	respLine, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("failed reading proxy response: %v", err)
	}
	if !strings.Contains(respLine, "200 Connection Established") {
		t.Fatalf("expected 200 Connection Established, got: %s", respLine)
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}

	// Send data through the established tunnel
	msg := "tunnel-ping-1234"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed reading echo through tunnel: %v", err)
	}
	if string(buf) != msg {
		t.Fatalf("expected echo %q, got %q", msg, string(buf))
	}
}

func TestProxy_AllowlistRulesAndRefusals(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "proxy.sock")
	res := &mockResolver{
		mapping: map[string][]net.IP{
			"api.anthropic.com":    {net.ParseIP("93.184.216.34")},
			"sub.google.com":       {net.ParseIP("142.250.180.206")},
			"google.com":           {net.ParseIP("142.250.180.206")},
			"rebind-loopback.test": {net.ParseIP("127.0.0.1")},
			"rebind-private.test":  {net.ParseIP("192.168.1.100")},
			"rebind-cgnat.test":    {net.ParseIP("100.65.0.1")},
			"rebind-ula.test":      {net.ParseIP("fd00::1")},
			"other.com":            {net.ParseIP("93.184.216.35")},
		},
	}

	serverIP := "100.85.111.88"
	serverPort := 8280
	pr, err := StartSandboxProxy(ProxyConfig{
		Mode: "filtered",
		Allow: []string{
			"api.anthropic.com:443",
			"*.google.com:443",
		},
		Server:   fmt.Sprintf("http://%s:%d", serverIP, serverPort),
		SockPath: sockPath,
		Resolver: res,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()

	// 1. Refused host (not in allowlist)
	conn := dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT other.com:443 HTTP/1.1\r\nHost: other.com:443\r\n\r\n")
	resp, _ := bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for other.com, got: %s", resp)
	}

	// 2. Refused port (host in allowlist, port is 80 instead of 443)
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT api.anthropic.com:80 HTTP/1.1\r\nHost: api.anthropic.com:80\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for wrong port, got: %s", resp)
	}

	// 3. Refused IP literal
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT 93.184.216.34:443 HTTP/1.1\r\nHost: 93.184.216.34:443\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for IP literal, got: %s", resp)
	}

	// 4. Refused DNS rebind to loopback
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT rebind-loopback.test:443 HTTP/1.1\r\nHost: rebind-loopback.test:443\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for loopback rebind, got: %s", resp)
	}

	// 5. Refused DNS rebind to private RFC1918
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT rebind-private.test:443 HTTP/1.1\r\nHost: rebind-private.test:443\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for private RFC1918 rebind, got: %s", resp)
	}

	// 6. Refused DNS rebind to CGNAT
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT rebind-cgnat.test:443 HTTP/1.1\r\nHost: rebind-cgnat.test:443\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for CGNAT rebind, got: %s", resp)
	}

	// 7. Refused DNS rebind to ULA
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "CONNECT rebind-ula.test:443 HTTP/1.1\r\nHost: rebind-ula.test:443\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for ULA rebind, got: %s", resp)
	}

	// 8. Plain HTTP to non-server host is refused
	conn = dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "GET http://api.anthropic.com/ HTTP/1.1\r\nHost: api.anthropic.com\r\n\r\n")
	resp, _ = bufio.NewReader(conn).ReadString('\n')
	_ = conn.Close()
	if !strings.Contains(resp, "403 Forbidden") {
		t.Fatalf("expected 403 for plain HTTP to non-server, got: %s", resp)
	}

	// 9. Wildcard *.google.com:443 matches sub.google.com:443
	if !pr.checkAllowed("sub.google.com", 443, true) {
		t.Error("expected *.google.com to allow sub.google.com:443")
	}
	// But not apex google.com (rule is *.google.com)
	if pr.checkAllowed("google.com", 443, true) {
		t.Error("expected *.google.com to NOT match google.com apex")
	}

	// 10. Server address allowed (even though it's CGNAT)
	serverIPNet := net.ParseIP(serverIP)
	if !pr.isServerIP(serverIPNet, serverPort) {
		t.Errorf("expected server IP %s:%d to be recognized as server", serverIP, serverPort)
	}
}

func TestProxy_PlainHTTPOnlyToServer(t *testing.T) {
	httpL, httpPort := startHTTPServer(t, "colonyhall-server-response")
	defer func() { _ = httpL.Close() }()

	sockPath := filepath.Join(t.TempDir(), "proxy.sock")
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	pr, err := StartSandboxProxy(ProxyConfig{
		Mode:        "filtered",
		Server:      serverURL,
		SockPath:    sockPath,
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()

	// 1. Plain HTTP to server succeeds
	conn := dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn, "GET http://127.0.0.1:%d/api/v1/ping HTTP/1.1\r\nHost: 127.0.0.1:%d\r\n\r\n", httpPort, httpPort)
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	_ = conn.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "colonyhall-server-response") {
		t.Fatalf("unexpected server response: %d, body: %s", resp.StatusCode, string(body))
	}

	// 2. Plain HTTP to any other host gets 403
	conn2 := dialProxyUnix(t, sockPath)
	_, _ = fmt.Fprintf(conn2, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n")
	line, _ := bufio.NewReader(conn2).ReadString('\n')
	_ = conn2.Close()
	if !strings.Contains(line, "403 Forbidden") {
		t.Fatalf("expected 403 for plain HTTP to other host, got: %s", line)
	}
}

func TestProxy_LogMode(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "proxy.sock")
	pr, err := StartSandboxProxy(ProxyConfig{
		Mode:     "log",
		Allow:    nil, // empty allowlist!
		SockPath: sockPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()

	// In log mode, checkAllowed returns true and records the target in seenHosts
	allowed := pr.checkAllowed("api.example.com", 443, true)
	if !allowed {
		t.Fatal("expected checkAllowed to return true in log mode")
	}

	pr.mu.Lock()
	seen := pr.seenHosts["api.example.com:443"]
	pr.mu.Unlock()
	if !seen {
		t.Fatal("expected api.example.com:443 to be recorded in seenHosts in log mode")
	}
}
