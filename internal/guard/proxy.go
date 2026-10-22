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
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	_, cgnatCIDR, _ = net.ParseCIDR("100.64.0.0/10")
	_, ulaCIDR, _   = net.ParseCIDR("fc00::/7")
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

// isBlockedIP reports whether ip is in private, loopback, link-local, multicast,
// unspecified, CGNAT (100.64.0.0/10), or ULA (fc00::/7) ranges.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if cgnatCIDR != nil && cgnatCIDR.Contains(ip) {
		return true
	}
	if ulaCIDR != nil && ulaCIDR.Contains(ip) {
		return true
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

	if !p.checkAllowed(host, port, true) {
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
	targetConn, err := dial(ctx, "tcp", net.JoinHostPort(host, portStr))
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
	if !p.isServer(host, port) {
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
	targetConn, err := dial(ctx, "tcp", net.JoinHostPort(host, portStr))
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

func (p *SandboxProxy) checkAllowed(host string, port int, isConnect bool) bool {
	host = strings.ToLower(host)
	portStr := strconv.Itoa(port)

	if p.Mode == "log" {
		p.logSeen(host, portStr)
		return true
	}

	// 1. Check against allowlist
	allowed := false
	if p.isServer(host, port) {
		allowed = true
	} else {
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
					allowed = true
					break
				}
			} else if rHost == host {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		return false
	}

	// 2. IP literal check
	if ip := net.ParseIP(host); ip != nil {
		return p.isServerIP(ip, port)
	}

	// 3. DNS resolution & private IP check
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false
	}

	for _, addr := range addrs {
		if isBlockedIP(addr.IP) {
			if !p.isServerIP(addr.IP, port) {
				return false
			}
		}
	}

	return true
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
