package guard

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSandboxNetwork_RealBwrap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second) // 30 s timed out under -race on a GitHub runner
	defer cancel()

	rootsDir := t.TempDir()
	homeDir := filepath.Join(rootsDir, "home")
	_ = os.MkdirAll(homeDir, 0o755)
	sbDir := filepath.Join(rootsDir, "sb")
	_ = os.MkdirAll(sbDir, 0o755)

	sb := DetectSandbox(ctx, homeDir, sbDir)
	if sb == nil || !sb.HasUnshareNet {
		t.Skip("bwrap with --unshare-net is not available or working; skipping real bwrap network test")
	}

	// 1. Compile colony binary for the sandbox bridge. The build gets its own
	// deadline: with a cold build cache (new Go version, -race job) it alone
	// can take longer than the sandbox checks below.
	colonyBin := filepath.Join(t.TempDir(), "colony")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer buildCancel()
	buildCmd := exec.CommandContext(buildCtx, "go", "build", "-o", colonyBin, "colonyhall/cmd/colony")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build colony binary: %v\n%s", err, string(out))
	}
	sb.ColonyBin = colonyBin
	sb.NetworkMode = "filtered"

	// 2. Start a local HTTP test server
	testL, testPort := startHTTPServer(t, "allowed-local-test-server-ok")
	defer func() { _ = testL.Close() }()

	// 3. Set up agent dir and start proxy on unix socket
	agentDir := filepath.Join(sbDir, "net-agent")
	_ = os.MkdirAll(filepath.Join(agentDir, "work"), 0o755)
	sockPath := filepath.Join(agentDir, "proxy.sock")

	res := &mockResolver{
		mapping: map[string][]net.IP{
			"allowed.test":    {net.ParseIP("93.184.216.34")},
			"notallowed.test": {net.ParseIP("93.184.216.35")},
		},
	}

	proxy, err := StartSandboxProxy(ProxyConfig{
		Mode:     "filtered",
		Allow:    []string{fmt.Sprintf("allowed.test:%d", testPort)},
		SockPath: sockPath,
		Resolver: res,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("tcp", testL.Addr().String())
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proxy.Close() }()

	// 4. Inside the sandbox, direct TCP connect to a public IP fails
	directScript := "curl --noproxy '*' -s --connect-timeout 1 http://8.8.8.8/ >/dev/null 2>&1 && echo direct_connected || echo direct_failed"
	bwrapArgs, env, err := sb.Command("claude-code", agentDir, filepath.Join(agentDir, "work"), nil, "bash", []string{"-c", directScript})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(ctx, sb.Bwrap, bwrapArgs...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap direct connect test failed to run: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "direct_failed") {
		t.Fatalf("expected direct connect to fail inside isolated sandbox, got: %s", string(out))
	}

	// 5. Request through the proxy to allowed test server works (via CONNECT tunnel: curl -p -x http://127.0.0.1:3128)
	allowedScript := fmt.Sprintf("curl -s -p -x http://127.0.0.1:3128 http://allowed.test:%d/", testPort)
	bwrapArgs, env, err = sb.Command("claude-code", agentDir, filepath.Join(agentDir, "work"), nil, "bash", []string{"-c", allowedScript})
	if err != nil {
		t.Fatal(err)
	}

	cmd = exec.CommandContext(ctx, sb.Bwrap, bwrapArgs...)
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap allowed request failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "allowed-local-test-server-ok") {
		t.Fatalf("expected proxy tunnel response 'allowed-local-test-server-ok', got: %s", string(out))
	}

	// 6. Request through the proxy to a not-allowed host gets 403
	forbiddenScript := fmt.Sprintf("curl -s -i -p -x http://127.0.0.1:3128 http://notallowed.test:%d/ | grep '403 Forbidden'", testPort)
	bwrapArgs, env, err = sb.Command("claude-code", agentDir, filepath.Join(agentDir, "work"), nil, "bash", []string{"-c", forbiddenScript})
	if err != nil {
		t.Fatal(err)
	}

	cmd = exec.CommandContext(ctx, sb.Bwrap, bwrapArgs...)
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap forbidden request failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "403 Forbidden") {
		t.Fatalf("expected 403 Forbidden for not-allowed host, got: %s", string(out))
	}

	// 7. Request to 127.0.0.1:3129 reaches the webread MCP server through the bridge
	webreadSock := filepath.Join(agentDir, "webread.sock")
	mcpSrv, err := StartWebReadMCPServer(webreadSock, FetchOptions{}, "@bwrap-agent", nil)
	if err != nil {
		t.Fatalf("start webread mcp server: %v", err)
	}
	defer func() { _ = mcpSrv.Close() }()

	webreadScript := "curl -s -i http://127.0.0.1:3129/mcp"
	bwrapArgs, env, err = sb.CommandWithOptions("claude-code", agentDir, filepath.Join(agentDir, "work"), nil, "bash", []string{"-c", webreadScript}, SandboxOptions{AllowWebRead: true})
	if err != nil {
		t.Fatal(err)
	}

	cmd = exec.CommandContext(ctx, sb.Bwrap, bwrapArgs...)
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap webread request failed: %v\n%s", err, string(out))
	}
	// The Streamable HTTP handler responds to GET with 400 or SSE or session response
	if !strings.Contains(string(out), "HTTP/1.1") {
		t.Fatalf("expected HTTP response from 127.0.0.1:3129 bridge, got: %s", string(out))
	}
}
