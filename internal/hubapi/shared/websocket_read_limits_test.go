package shared

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketReadLimitClassesPreserveLargeDesktopFrames(t *testing.T) {
	t.Parallel()

	const uncompressed4KRGBABytes int64 = 3840 * 2160 * 4
	if MaxUpstreamDesktopMessageBytes < uncompressed4KRGBABytes {
		t.Fatalf("desktop limit %d is smaller than a 4K RGBA frame (%d)", MaxUpstreamDesktopMessageBytes, uncompressed4KRGBABytes)
	}
	if !(MaxBrowserControlMessageBytes < MaxBrowserInteractiveMessageBytes &&
		MaxBrowserInteractiveMessageBytes < MaxUpstreamTerminalMessageBytes &&
		MaxUpstreamTerminalMessageBytes < MaxUpstreamDesktopMessageBytes) {
		t.Fatalf(
			"read limits are not input-specific: control=%d interactive=%d terminal=%d desktop=%d",
			MaxBrowserControlMessageBytes,
			MaxBrowserInteractiveMessageBytes,
			MaxUpstreamTerminalMessageBytes,
			MaxUpstreamDesktopMessageBytes,
		)
	}
}

func TestBrowserEventsReadLimitAlwaysRetainsSecureCeiling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured int64
		want       int64
	}{
		{name: "zero uses default", configured: 0, want: MaxBrowserControlMessageBytes},
		{name: "negative uses default", configured: -1, want: MaxBrowserControlMessageBytes},
		{name: "excessive uses default", configured: MaxBrowserControlMessageBytes + 1, want: MaxBrowserControlMessageBytes},
		{name: "smaller override", configured: 4096, want: 4096},
		{name: "default accepted", configured: MaxBrowserControlMessageBytes, want: MaxBrowserControlMessageBytes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := browserEventsReadLimit(test.configured); got != test.want {
				t.Fatalf("browserEventsReadLimit(%d) = %d, want %d", test.configured, got, test.want)
			}
		})
	}
}

func TestWebSocketReadLimitRejectsOversizedFragmentedMessageAndCloses(t *testing.T) {
	serverConn, clientConn, cleanup := newWebSocketPair(t)
	defer cleanup()

	const limit = int64(128)
	setWebSocketReadLimit(serverConn, limit)

	readErr := make(chan error, 1)
	go func() {
		_, _, err := serverConn.ReadMessage()
		readErr <- err
	}()

	writeMaskedClientFrame(t, clientConn.UnderlyingConn(), false, websocket.BinaryMessage, bytes.Repeat([]byte("a"), 60), 1)
	writeMaskedClientFrame(t, clientConn.UnderlyingConn(), false, 0, bytes.Repeat([]byte("b"), 60), 2)
	writeMaskedClientFrame(t, clientConn.UnderlyingConn(), true, 0, bytes.Repeat([]byte("c"), 9), 3)

	select {
	case err := <-readErr:
		if !errors.Is(err, websocket.ErrReadLimit) {
			t.Fatalf("ReadMessage() error = %v, want ErrReadLimit", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("oversized fragmented message did not terminate the read")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := clientConn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("client close error = %v, want close code %d", err, websocket.CloseMessageTooBig)
	}
}

func TestWebSocketReadLimitAllowsBinaryFragmentedMessageAtLimit(t *testing.T) {
	serverConn, clientConn, cleanup := newWebSocketPair(t)
	defer cleanup()

	const limit = int64(128)
	setWebSocketReadLimit(serverConn, limit)

	result := make(chan struct {
		messageType int
		payload     []byte
		err         error
	}, 1)
	go func() {
		messageType, payload, err := serverConn.ReadMessage()
		result <- struct {
			messageType int
			payload     []byte
			err         error
		}{messageType: messageType, payload: payload, err: err}
	}()

	want := append(bytes.Repeat([]byte("a"), 60), bytes.Repeat([]byte("b"), 60)...)
	want = append(want, bytes.Repeat([]byte("c"), 8)...)
	writeMaskedClientFrame(t, clientConn.UnderlyingConn(), false, websocket.BinaryMessage, want[:60], 4)
	writeMaskedClientFrame(t, clientConn.UnderlyingConn(), false, 0, want[60:120], 5)
	writeMaskedClientFrame(t, clientConn.UnderlyingConn(), true, 0, want[120:], 6)

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("ReadMessage() error = %v", got.err)
		}
		if got.messageType != websocket.BinaryMessage {
			t.Fatalf("message type = %d, want binary", got.messageType)
		}
		if !bytes.Equal(got.payload, want) {
			t.Fatalf("payload length = %d, want %d", len(got.payload), len(want))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("valid fragmented binary message was not delivered")
	}
}

// writeMaskedClientFrame writes one RFC 6455 client frame directly so tests
// deterministically exercise continuation frames rather than relying on a
// WebSocket implementation's buffering heuristics.
func writeMaskedClientFrame(t *testing.T, conn net.Conn, final bool, opcode byte, payload []byte, maskSeed byte) {
	t.Helper()
	if len(payload) > 125 {
		t.Fatalf("test frame payload %d exceeds short-frame encoding", len(payload))
	}

	first := opcode
	if final {
		first |= 0x80
	}
	mask := [4]byte{maskSeed, maskSeed + 1, maskSeed + 2, maskSeed + 3}
	frame := make([]byte, 6+len(payload))
	frame[0] = first
	frame[1] = 0x80 | byte(len(payload))
	copy(frame[2:6], mask[:])
	for i, value := range payload {
		frame[6+i] = value ^ mask[i%len(mask)]
	}

	for len(frame) > 0 {
		n, err := conn.Write(frame)
		if err != nil {
			t.Fatalf("write fragmented client frame: %v", err)
		}
		frame = frame[n:]
	}
}
