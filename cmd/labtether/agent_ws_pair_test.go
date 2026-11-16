package main

import (
	"testing"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// createWSPairForNetworkTest: one implementation, in testutil.
func createWSPairForNetworkTest(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	return testutil.CreateWSPairForNetworkTest(t)
}
