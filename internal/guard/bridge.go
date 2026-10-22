package guard

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
)

// BridgeServer listens on 127.0.0.1:<port> and forwards TCP connections to a Unix domain socket.
type BridgeServer struct {
	SockPath string
	Port     int
	listener net.Listener
	closed   chan struct{}
	closeMu  sync.Mutex
}

// StartBridge starts listening on 127.0.0.1:port and forwarding to sockPath.
func StartBridge(sockPath string, port int) (*BridgeServer, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bridge: listen %s: %w", addr, err)
	}
	b := &BridgeServer{
		SockPath: sockPath,
		Port:     port,
		listener: l,
		closed:   make(chan struct{}),
	}
	go b.serve()
	return b, nil
}

func (b *BridgeServer) Addr() net.Addr {
	return b.listener.Addr()
}

func (b *BridgeServer) serve() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			select {
			case <-b.closed:
				return
			default:
				return
			}
		}
		go b.handleConn(conn)
	}
}

func (b *BridgeServer) handleConn(tcpConn net.Conn) {
	uConn, err := net.Dial("unix", b.SockPath)
	if err != nil {
		_ = tcpConn.Close()
		return
	}

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
	go half(uConn, tcpConn)
	go half(tcpConn, uConn)
	wg.Wait()
	_ = tcpConn.Close()
	_ = uConn.Close()
}

// Close stops the bridge listener.
func (b *BridgeServer) Close() error {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	select {
	case <-b.closed:
		return nil
	default:
		close(b.closed)
		return b.listener.Close()
	}
}

// RunBridgeCommand starts the bridge and runs the target command, forwarding stdio.
// When the command exits, the bridge is closed and the process exit code or error is returned.
func RunBridgeCommand(ctx context.Context, sockPath string, port int, clientBin string, clientArgs []string) error {
	bridge, err := StartBridge(sockPath, port)
	if err != nil {
		return err
	}
	defer func() { _ = bridge.Close() }()

	cmd := exec.CommandContext(ctx, clientBin, clientArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}
