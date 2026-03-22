package main

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

func startBrowserWebSocketKeepalive(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func() {
	return shared.StartBrowserWebSocketKeepalive(wsConn, writeMu, streamLabel)
}

func touchBrowserWebSocketReadDeadline(wsConn *websocket.Conn) error {
	return shared.TouchBrowserWebSocketReadDeadline(wsConn)
}
