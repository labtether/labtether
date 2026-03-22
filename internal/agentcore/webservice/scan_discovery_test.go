package webservice

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestProbeTCPPortReachable_TLSHelloWritesData(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	readLenCh := make(chan int, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			readLenCh <- -1
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		var buf [1]byte
		n, readErr := conn.Read(buf[:])
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			readLenCh <- -2
			return
		}
		readLenCh <- n
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if !probeTCPPortReachable(ctx, ln.Addr().String(), true) {
		t.Fatal("expected probe to mark port reachable")
	}

	select {
	case n := <-readLenCh:
		if n <= 0 {
			t.Fatalf("expected TLS probe to write bytes, got %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for listener read")
	}
}

func TestProbeTCPPortReachable_PlainTCPWritesNoData(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	readLenCh := make(chan int, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			readLenCh <- -1
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		var buf [1]byte
		n, readErr := conn.Read(buf[:])
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			readLenCh <- -2
			return
		}
		readLenCh <- n
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if !probeTCPPortReachable(ctx, ln.Addr().String(), false) {
		t.Fatal("expected probe to mark port reachable")
	}

	select {
	case n := <-readLenCh:
		if n != 0 {
			t.Fatalf("expected plain TCP probe to close without payload, got %d byte(s)", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for listener read")
	}
}
