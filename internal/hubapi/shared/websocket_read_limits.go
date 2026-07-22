package shared

import "github.com/gorilla/websocket"

const (
	// MaxBrowserControlMessageBytes covers signaling and sideband control
	// messages while preventing authenticated clients from assembling
	// unbounded fragmented messages in memory.
	MaxBrowserControlMessageBytes int64 = 64 << 10

	// MaxBrowserInteractiveMessageBytes matches the established terminal and
	// desktop input ceiling. It leaves room for clipboard/control payloads while
	// bounding browser-originated terminal and remote-desktop input.
	MaxBrowserInteractiveMessageBytes int64 = 256 << 10

	// MaxUpstreamTerminalMessageBytes allows generously batched shell output
	// from connector WebSockets without accepting unbounded upstream messages.
	MaxUpstreamTerminalMessageBytes int64 = 4 << 20

	// MaxUpstreamDesktopMessageBytes accommodates large binary VNC frames,
	// including an uncompressed 4K RGBA frame, while bounding upstream memory.
	MaxUpstreamDesktopMessageBytes int64 = 32 << 20
)

func setWebSocketReadLimit(conn *websocket.Conn, maxBytes int64) {
	if conn == nil {
		return
	}
	conn.SetReadLimit(maxBytes)
}

// LimitBrowserControlMessages bounds small browser-originated control streams.
func LimitBrowserControlMessages(conn *websocket.Conn) {
	setWebSocketReadLimit(conn, MaxBrowserControlMessageBytes)
}

// LimitBrowserInteractiveMessages bounds browser terminal/desktop input.
func LimitBrowserInteractiveMessages(conn *websocket.Conn) {
	setWebSocketReadLimit(conn, MaxBrowserInteractiveMessageBytes)
}

// LimitUpstreamTerminalMessages bounds terminal output from connector WebSockets.
func LimitUpstreamTerminalMessages(conn *websocket.Conn) {
	setWebSocketReadLimit(conn, MaxUpstreamTerminalMessageBytes)
}

// LimitUpstreamDesktopMessages bounds binary desktop output from upstreams.
func LimitUpstreamDesktopMessages(conn *websocket.Conn) {
	setWebSocketReadLimit(conn, MaxUpstreamDesktopMessageBytes)
}
