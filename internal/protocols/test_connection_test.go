package protocols

import (
	"context"
	"net"
	"testing"
)

func TestRDPReportsGuacdReachableOnlyWhenDialSucceeds(t *testing.T) {
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer targetListener.Close()
	go acceptAndCloseOnce(t, targetListener)

	guacdListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen guacd: %v", err)
	}
	defer guacdListener.Close()
	go acceptAndCloseOnce(t, guacdListener)

	result := TestRDP(
		context.Background(),
		targetListener.Addr().(*net.TCPAddr).IP.String(),
		targetListener.Addr().(*net.TCPAddr).Port,
		guacdListener.Addr().String(),
	)
	if !result.Success {
		t.Fatalf("expected success, got error=%q", result.Error)
	}
	if result.Message != "reachable (guacd reachable)" {
		t.Fatalf("unexpected message: %q", result.Message)
	}
}

func TestRDPFailsWhenGuacdDialFails(t *testing.T) {
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer targetListener.Close()
	go acceptAndCloseOnce(t, targetListener)

	unusedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen unused: %v", err)
	}
	guacdAddr := unusedListener.Addr().String()
	unusedListener.Close()

	result := TestRDP(
		context.Background(),
		targetListener.Addr().(*net.TCPAddr).IP.String(),
		targetListener.Addr().(*net.TCPAddr).Port,
		guacdAddr,
	)
	if result.Success {
		t.Fatalf("expected failure when guacd is unreachable, got message=%q", result.Message)
	}
	if result.Error == "" {
		t.Fatal("expected guacd error message")
	}
}

func acceptAndCloseOnce(t *testing.T, listener net.Listener) {
	t.Helper()
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	conn.Close()
}
