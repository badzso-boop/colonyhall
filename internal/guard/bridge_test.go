package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestBridge_BidirectionalTrafficAndCleanClose(t *testing.T) {
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "test.sock")

	// 1. Unix socket echo server
	uListener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = uListener.Close() }()

	go func() {
		for {
			conn, err := uListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	// 2. Start bridge on a random free local TCP port
	testPort := 0
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	testPort = l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	bridge, err := StartBridge(sockPath, testPort)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Connect TCP to bridge, send data, verify echo
	tcpConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", testPort))
	if err != nil {
		t.Fatal(err)
	}

	testMsg := "hello through bridge"
	if _, err := tcpConn.Write([]byte(testMsg)); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(testMsg))
	if _, err := io.ReadFull(tcpConn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != testMsg {
		t.Fatalf("expected echo %q, got %q", testMsg, string(buf))
	}
	_ = tcpConn.Close()

	// 4. Closes cleanly
	if err := bridge.Close(); err != nil {
		t.Fatalf("bridge close failed: %v", err)
	}

	// Check that port is no longer accepting
	time.Sleep(20 * time.Millisecond)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", testPort), 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected connection to fail after bridge closed")
	}
}

func TestBridge_RefusesWhenSocketGone(t *testing.T) {
	sockDir := t.TempDir()
	nonExistentSock := filepath.Join(sockDir, "missing.sock")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	testPort := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	bridge, err := StartBridge(nonExistentSock, testPort)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()

	// Dialing bridge should accept TCP then immediately close because unix socket is missing
	tcpConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", testPort))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tcpConn.Close() }()

	// Read should return EOF immediately
	buf := make([]byte, 10)
	n, err := tcpConn.Read(buf)
	if !errors.Is(err, io.EOF) && n != 0 {
		t.Fatalf("expected EOF when unix socket missing, got n=%d, err=%v", n, err)
	}
}

func TestBridge_RunBridgeCommand(t *testing.T) {
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "dummy.sock")

	// Create dummy unix socket listener
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ul.Close() }()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	testPort := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run simple command like echo or true
	err = RunBridgeCommand(ctx, sockPath, testPort, "echo", []string{"bridge-ok"})
	if err != nil {
		t.Fatalf("RunBridgeCommand failed: %v", err)
	}
}
