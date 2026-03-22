package shared

import "github.com/labtether/labtether/internal/agentmgr"

// WSHandler handles a WebSocket message from an agent connection.
type WSHandler func(conn *agentmgr.AgentConn, msg agentmgr.Message)

// WSRouter maps message types to handlers.
type WSRouter map[string]WSHandler
