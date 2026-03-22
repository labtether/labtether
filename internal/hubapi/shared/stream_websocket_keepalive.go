package shared

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	StreamBrowserPingInterval     = 20 * time.Second
	StreamBrowserReadDeadline     = 70 * time.Second
	StreamBrowserPingWriteTimeout = 10 * time.Second
)

// StartBrowserWebSocketKeepalive keeps browser stream sockets alive across
// idle LAN/proxy links by periodically sending WebSocket ping control frames.
// It also installs a pong handler to extend read deadlines on every pong.
func StartBrowserWebSocketKeepalive(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func() {
	if wsConn == nil {
		return func() {}
	}

	if err := TouchBrowserWebSocketReadDeadline(wsConn); err != nil && streamLabel != "" {
		log.Printf("%s: websocket keepalive initial deadline update failed: %v", streamLabel, err)
	}
	wsConn.SetPongHandler(func(_ string) error {
		return TouchBrowserWebSocketReadDeadline(wsConn)
	})

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			close(stopCh)
			_ = wsConn.SetReadDeadline(time.Time{})
		})
	}

	go func() {
		ticker := time.NewTicker(StreamBrowserPingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				deadline := time.Now().Add(StreamBrowserPingWriteTimeout)
				if writeMu != nil {
					writeMu.Lock()
				}
				err := wsConn.WriteControl(websocket.PingMessage, []byte("lt"), deadline)
				if writeMu != nil {
					writeMu.Unlock()
				}
				if err != nil {
					if streamLabel != "" {
						log.Printf("%s: websocket keepalive ping failed: %v", streamLabel, err)
					}
					_ = wsConn.Close()
					return
				}
			}
		}
	}()

	return stop
}

// TouchBrowserWebSocketReadDeadline extends the browser WebSocket read deadline.
// Call this on any successfully-read browser frame so active streams do not
// depend solely on websocket pong control frames for liveness.
func TouchBrowserWebSocketReadDeadline(wsConn *websocket.Conn) error {
	if wsConn == nil {
		return nil
	}
	return wsConn.SetReadDeadline(time.Now().Add(StreamBrowserReadDeadline))
}
