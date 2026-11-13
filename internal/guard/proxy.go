package guard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default allowed endpoints per AI client.
var (
	DefaultClaudeAllow = []string{
		"api.anthropic.com:443",
	}

	DefaultAntigravityAllow = []string{
		"daily-cloudcode-pa.googleapis.com:443",
		"cloudcode-pa.googleapis.com:443",
		"www.googleapis.com:443",
		"antigravity-unleash.goog:443",
		"generativelanguage.googleapis.com:443",
		"aicode.googleapis.com:443", // Antigravity calls it on every run (since 2026-10-26)
		"oauth2.googleapis.com:443",
		"accounts.google.com:443",
		"play.googleapis.com:443",
		"lh3.googleusercontent.com:443",
	}

	ClientDefaultAllow = map[string][]string{
		"claude-code": DefaultClaudeAllow,
		"antigravity": DefaultAntigravityAllow,
	}
)

// IPResolver resolves host names to IP addresses.
type IPResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// ProxyConfig defines the configuration for a sandboxed run's HTTP proxy.
type ProxyConfig struct {
	Mode        string // "filtered", "log", "open"
	Allow       []string
	Server      string
	Client      string
	AgentName   string
	SockPath    string
	Log         *slog.Logger
	Resolver    IPResolver
	DialTimeout time.Duration
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// SandboxProxy is an HTTP/CONNECT proxy running on a Unix domain socket
// for a sandboxed agent run.
type SandboxProxy struct {
	Mode        string
	Allow       []string
	ServerHost  string
	ServerPort  int
	ServerIPs   []net.IP
	AgentName   string
	SockPath    string
	Log         *slog.Logger
	Resolver    IPResolver
	DialTimeout time.Duration
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	listener net.Listener
	closed   chan struct{}
	closeMu  sync.Mutex

	mu           sync.Mutex
	refusedHosts map[string]bool
	seenHosts    map[string]bool
}

// blockedPrefixes are ranges no fetch may reach besides the ones the net
// package names (round 40): "this network", benchmarking, IETF protocol
// assignments, reserved and broadcast, CGNAT (Tailscale), ULA, and the IPv6
// forms that carry an IPv4 address inside (NAT64, 6to4, Teredo), which
// could otherwise reach a private IPv4 address through a translator.
var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4",
		"fc00::/7", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32"} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// isBlockedIP reports whether ip is anything but a public unicast address:
// private, loopback, link-local, multicast, unspecified, or one of
// blockedPrefixes. An IPv4-mapped IPv6 address is checked as IPv4.
func isBlockedIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// StartSandboxProxy creates and starts a proxy listening on cfg.SockPath.
func StartSandboxProxy(cfg ProxyConfig) (*SandboxProxy, error) {
	if cfg.SockPath == "" {
		return nil, errors.New("proxy: SockPath is required")
	}
	_ = os.Remove(cfg.SockPath)

	l, err := net.Listen("unix", cfg.SockPath)
	if err != nil {
		return nil, fmt.Errorf("proxy: listen unix %s: %w", cfg.SockPath, err)
	}
	// Allow socket access inside the sandbox
	_ = os.Chmod(cfg.SockPath, 0o666)

	p := &SandboxProxy{
		Mode:         cfg.Mode,
		Allow:        cfg.Allow,
		AgentName:    cfg.AgentName,
		SockPath:     cfg.SockPath,
		Log:          cfg.Log,
		Resolver:     cfg.Resolver,
		DialTimeout:  cfg.DialTimeout,
		DialContext:  cfg.DialContext,
		listener:     l,
		closed:       make(chan struct{}),
		refusedHosts: make(map[string]bool),
		seenHosts:    make(map[string]bool),
	}
	if p.Mode == "" {
		p.Mode = "filtered"
	}
	if p.DialTimeout == 0 {
		p.DialTimeout = 10 * time.Second
	}

	p.initServerInfo(cfg.Server)

	go p.serve()
	return p, nil
}

func (p *SandboxProxy) initServerInfo(serverURL string) {
	if serverURL == "" {
		return
	}
	u, err := url.Parse(serverURL)
	if err != nil {
		return
	}
	h := u.Hostname()
	pStr := u.Port()
	if pStr == "" {
		if strings.EqualFold(u.Scheme, "https") {
			pStr = "443"
		} else {
			pStr = "80"
		}
	}
	port, _ := strconv.Atoi(pStr)
	p.ServerHost = strings.ToLower(h)
	p.ServerPort = port

	if ip := net.ParseIP(h); ip != nil {
		p.ServerIPs = []net.IP{ip}
	} else if p.Resolver != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if addrs, err := p.Resolver.LookupIPAddr(ctx, h); err == nil {
			for _, a := range addrs {
				p.ServerIPs = append(p.ServerIPs, a.IP)
			}
		}
	} else {
		if ips, err := net.LookupIP(h); err == nil {
			p.ServerIPs = ips
		}
	}
}

func (p *SandboxProxy) isServer(host string, port int) bool {
	if p.ServerHost == "" {
		return false
	}
	if strings.EqualFold(host, p.ServerHost) && port == p.ServerPort {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return p.isServerIP(ip, port)
	}
	return false
}

func (p *SandboxProxy) isServerIP(ip net.IP, port int) bool {
	if port != p.ServerPort {
		return false
	}
	for _, sip := range p.ServerIPs {
		if sip.Equal(ip) {
			return true
		}
	}
	return false
}

func (p *SandboxProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.closed:
				return
			default:
				return
			}
		}
		go p.handleConn(conn)
	}
}

func (p *SandboxProxy) handleConn(conn net.Conn) {
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		_ = conn.Close()
		return
	}

	if req.Method == http.MethodConnect {
		p.handleConnect(conn, req.RequestURI)
		return
	}

	p.handlePlainHTTP(conn, req)
}

func (p *SandboxProxy) handleConnect(clientConn net.Conn, target string) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		_, _ = clientConn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		_ = clientConn.Close()
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		_, _ = clientConn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		_ = clientConn.Close()
		return
	}

	ip, ok := p.checkAllowed(host, port)
	if !ok {
		p.logRefusal(host, portStr)
		_, _ = clientConn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Type: text/plain\r\n\r\nForbidden\n"))
		_ = clientConn.Close()
		return
	}

	dial := p.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.DialTimeout)
	defer cancel()
	// Dial the address that was checked, never the name again: a second
	// DNS answer could point at a private address (DNS rebinding).
	targetConn, err := dial(ctx, "tcp", net.JoinHostPort(ip.String(), portStr))
	if err != nil {
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		_ = clientConn.Close()
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		_ = clientConn.Close()
		_ = targetConn.Close()
		return
	}

	spliceConns(clientConn, targetConn)
}

func (p *SandboxProxy) handlePlainHTTP(clientConn net.Conn, req *http.Request) {
	host := req.URL.Hostname()
	if host == "" {
		host = req.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	portStr := req.URL.Port()
	if portStr == "" {
		if strings.Contains(req.Host, ":") {
			_, portStr, _ = net.SplitHostPort(req.Host)
		}
	}
	if portStr == "" {
		if strings.EqualFold(req.URL.Scheme, "https") {
			portStr = "443"
		} else {
			portStr = "80"
		}
	}
	port, _ := strconv.Atoi(portStr)

	// In log mode, log seen host:port
	if p.Mode == "log" {
		p.logSeen(host, portStr)
	}

	// Plain HTTP is ONLY allowed to the Colonyhall server
	ip, ok := p.checkAllowed(host, port)
	if !p.isServer(host, port) || !ok {
		p.logRefusal(host, portStr)
		resp := "HTTP/1.1 403 Forbidden\r\nContent-Type: text/plain\r\n\r\nPlain HTTP is only allowed to the Colonyhall server\n"
		_, _ = clientConn.Write([]byte(resp))
		_ = clientConn.Close()
		return
	}

	dial := p.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.DialTimeout)
	defer cancel()
	targetConn, err := dial(ctx, "tcp", net.JoinHostPort(ip.String(), portStr))
	if err != nil {
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		_ = clientConn.Close()
		return
	}

	if err := req.Write(targetConn); err != nil {
		_ = targetConn.Close()
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		_ = clientConn.Close()
		return
	}

	spliceConns(clientConn, targetConn)
}

// checkAllowed decides whether host:port may be reached and returns the
// checked address to dial. Log mode skips the allowlist (to discover what a
// client needs) but never the address checks: private, loopback and the other
// internal ranges stay closed in every mode, except the Colonyhall server.
func (p *SandboxProxy) checkAllowed(host string, port int) (net.IP, bool) {
	host = strings.ToLower(host)
	portStr := strconv.Itoa(port)

	if p.Mode == "log" {
		p.logSeen(host, portStr)
	} else if !p.isServer(host, port) && !p.inAllowlist(host, port) {
		return nil, false
	}

	if ip := net.ParseIP(host); ip != nil {
		if p.isServerIP(ip, port) {
			return ip, true
		}
		return nil, false // IP literals only for the server
	}

	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, false
	}
	for _, addr := range addrs {
		if isBlockedIP(addr.IP) && !p.isServerIP(addr.IP, port) {
			return nil, false
		}
	}
	return addrs[0].IP, true
}

func (p *SandboxProxy) inAllowlist(host string, port int) bool {
	for _, rule := range p.Allow {
		rHost, rPortStr, err := net.SplitHostPort(rule)
		if err != nil {
			continue
		}
		rPort, err := strconv.Atoi(rPortStr)
		if err != nil || rPort != port {
			continue
		}
		rHost = strings.ToLower(rHost)
		if strings.HasPrefix(rHost, "*.") {
			suffix := rHost[1:] // e.g. ".google.com"
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		} else if rHost == host {
			return true
		}
	}
	return false
}

func (p *SandboxProxy) logSeen(host, port string) {
	target := net.JoinHostPort(strings.ToLower(host), port)
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.seenHosts[target] {
		p.seenHosts[target] = true
		if p.Log != nil {
			p.Log.Info("guard: sandbox network seen", "agent", p.AgentName, "target", target)
		}
	}
}

func (p *SandboxProxy) logRefusal(host, port string) {
	target := net.JoinHostPort(strings.ToLower(host), port)
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.refusedHosts[target] {
		p.refusedHosts[target] = true
		if p.Log != nil {
			p.Log.Warn("guard: sandbox network refused", "agent", p.AgentName, "target", target)
		}
	}
}

func (p *SandboxProxy) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	select {
	case <-p.closed:
		return nil
	default:
		close(p.closed)
		err := p.listener.Close()
		_ = os.Remove(p.SockPath)
		return err
	}
}

func spliceConns(c1, c2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	half := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		} else if uc, ok := dst.(*net.UnixConn); ok {
			_ = uc.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	go half(c1, c2)
	go half(c2, c1)
	wg.Wait()
	_ = c1.Close()
	_ = c2.Close()
}
