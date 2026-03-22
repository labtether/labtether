package terminal

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	telnetDialTimeout = 6 * time.Second

	// IAC command bytes.
	iacByte = 0xFF
	iacWill = 0xFB
	iacWont = 0xFC
	iacDo   = 0xFD
	iacDont = 0xFE
	iacSB   = 0xFA // sub-negotiation begin
	iacSE   = 0xF0 // sub-negotiation end

	// Well-known option codes.
	optEcho = 0x01 // ECHO
	optSGA  = 0x03 // Suppress Go Ahead
)

// HandleTelnetStream bridges a WebSocket connection to a raw TCP Telnet endpoint.
// It performs RFC 854 IAC negotiation and transparently forwards data between
// the browser and the Telnet server.
func (d *Deps) HandleTelnetStream(w http.ResponseWriter, r *http.Request, session terminal.Session, host string, port int) {
	traceID := shared.BrowserStreamTraceID(r)
	traceLog := shared.StreamTraceLogValue(traceID)
	logContext := fmt.Sprintf("session=%s target=%s:%d trace=%s", session.ID, host, port, traceLog)

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	tcpConn, err := net.DialTimeout("tcp", addr, telnetDialTimeout) // #nosec G704 -- Telnet target comes from resolved protocol config, not raw browser input.
	if err != nil {
		log.Printf("terminal-telnet: connect_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local dial errors.
		http.Error(w, "telnet connection failed", http.StatusBadGateway)
		return
	}
	defer tcpConn.Close()

	wsConn, err := d.TerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal-telnet: upgrade_failed %s err=%v", logContext, err) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local upgrade errors.
		return
	}
	defer wsConn.Close()
	wsConn.SetReadLimit(maxTerminalInputReadBytes)

	log.Printf("terminal-telnet: stream_connected %s", logContext) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }

	var writeMu sync.Mutex
	stopKeepalive := shared.StartBrowserWebSocketKeepalive(wsConn, &writeMu, "terminal-telnet:"+session.ID)
	defer stopKeepalive()

	// TCP -> WS: read from Telnet server, filter IAC sequences, write to browser.
	go func() {
		defer closeDone()
		buf := make([]byte, 4096)
		for {
			n, readErr := tcpConn.Read(buf)
			if n > 0 {
				filtered := filterIAC(tcpConn, buf[:n])
				if len(filtered) > 0 {
					writeMu.Lock()
					_ = wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					writeErr := wsConn.WriteMessage(websocket.BinaryMessage, filtered)
					writeMu.Unlock()
					if writeErr != nil {
						return
					}
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// WS -> TCP: read from browser, write to Telnet server.
	stopCloser := make(chan struct{})
	defer close(stopCloser)
	go func() {
		select {
		case <-done:
			_ = wsConn.SetReadDeadline(time.Now())
			_ = wsConn.Close()
		case <-stopCloser:
		}
	}()

	connectStart := time.Now()
	for {
		messageType, payload, readErr := wsConn.ReadMessage()
		if readErr != nil {
			log.Printf("terminal-telnet: stream_ended %s elapsed=%s", logContext, time.Since(connectStart).Round(time.Millisecond)) // #nosec G706 -- Log context is composed from bounded session/runtime identifiers.
			return
		}
		_ = shared.TouchBrowserWebSocketReadDeadline(wsConn)
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if _, writeErr := tcpConn.Write(payload); writeErr != nil {
			log.Printf("terminal-telnet: tcp_write_failed %s err=%v", logContext, writeErr) // #nosec G706 -- Log fields are bounded session/runtime identifiers and local socket errors.
			return
		}
	}
}

// filterIAC processes a Telnet IAC command sequence in buf, writes responses
// back to conn where appropriate (WILL/WONT/DO/DONT negotiation), and returns
// the data bytes with all IAC sequences stripped.
func filterIAC(conn net.Conn, buf []byte) []byte {
	out := make([]byte, 0, len(buf))
	i := 0
	for i < len(buf) {
		b := buf[i]
		if b != iacByte {
			out = append(out, b)
			i++
			continue
		}
		// Need at least IAC + command byte.
		if i+1 >= len(buf) {
			// Truncated IAC at end of buffer — consume the byte.
			i++
			continue
		}
		cmd := buf[i+1]
		switch cmd {
		case iacSB:
			// Sub-negotiation: skip from SB up to and including SE.
			i += 2 // skip IAC SB
			for i < len(buf) {
				if buf[i] == iacByte && i+1 < len(buf) && buf[i+1] == iacSE {
					i += 2 // skip IAC SE
					break
				}
				i++
			}
		case iacWill:
			// Server says WILL <opt>; respond appropriately.
			if i+2 < len(buf) {
				opt := buf[i+2]
				var resp []byte
				if opt == optEcho || opt == optSGA {
					// DO ECHO / DO SGA — accept these.
					resp = []byte{iacByte, iacDo, opt}
				} else {
					// DONT for everything else.
					resp = []byte{iacByte, iacDont, opt}
				}
				_, _ = conn.Write(resp)
			}
			i += 3
		case iacWont:
			// Server won't do something; no reply needed.
			i += 3
		case iacDo:
			// Server says DO <opt>; respond appropriately.
			if i+2 < len(buf) {
				opt := buf[i+2]
				var resp []byte
				if opt == optEcho || opt == optSGA {
					// WILL ECHO / WILL SGA — agree.
					resp = []byte{iacByte, iacWill, opt}
				} else {
					// WONT for everything we can't do.
					resp = []byte{iacByte, iacWont, opt}
				}
				_, _ = conn.Write(resp)
			}
			i += 3
		case iacDont:
			// Server says DONT <opt>; no reply needed.
			i += 3
		default:
			// Unknown or bare IAC — skip two bytes.
			i += 2
		}
	}
	return out
}
