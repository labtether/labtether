package truenas

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/securityruntime"
)

// globalRequestID is a process-wide monotonically increasing JSON-RPC request counter.
var globalRequestID atomic.Uint64

// wsConn is the subset of websocket.Conn used by Client.Call. It is an
// interface to allow deterministic transport failure tests.
type wsConn interface {
	SetWriteDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}

var dialWS = func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
	validatedEndpoint, err := securityruntime.ValidateOutboundURL(endpoint)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- user-controlled homelab setting for self-signed certs.
			InsecureSkipVerify: skipVerify, //nolint:gosec // #nosec G402 -- user-controlled homelab setting
		},
	}

	conn, _, err := dialer.DialContext(ctx, validatedEndpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Client is a connect-per-operation WebSocket JSON-RPC 2.0 client for TrueNAS.
//
// Each Call dials a fresh WebSocket connection, authenticates with the TrueNAS
// API key, sends the requested method, reads the response, and closes the
// connection. This keeps the client stateless and safe for concurrent use.
type Client struct {
	// BaseURL is the TrueNAS base URL, e.g. "https://truenas.local" or
	// "wss://truenas.local". http/https are automatically converted to ws/wss.
	BaseURL string

	// APIKey is the TrueNAS API key used with auth.login_with_api_key.
	APIKey string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.

	// SkipVerify disables TLS certificate verification. Useful for homelabs
	// using self-signed certificates.
	SkipVerify bool

	// Timeout is the per-operation deadline applied to each Call. Defaults to
	// 30 seconds when zero.
	Timeout time.Duration
}

// rpcRequest is a JSON-RPC 2.0 request envelope.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// rpcResponse is a JSON-RPC 2.0 response envelope.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcErrorBody   `json:"error"`
}

// rpcErrorBody is the "error" object inside a JSON-RPC 2.0 response.
type rpcErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

// RPCError is returned by Call when the TrueNAS server responds with a
// JSON-RPC error object.
type RPCError struct {
	Code    int
	Message string
	Reason  string
}

func (e *RPCError) Error() string {
	message := strings.TrimSpace(e.Message)
	reason := strings.TrimSpace(e.Reason)
	if reason != "" && !strings.Contains(strings.ToLower(message), strings.ToLower(reason)) {
		if message != "" {
			message = message + ": " + reason
		} else {
			message = reason
		}
	}
	if message == "" {
		message = "unknown error"
	}
	return fmt.Sprintf("truenas rpc error %d: %s", e.Code, message)
}

// IsMethodNotFound reports whether err is an RPCError with code -32601
// (Method not found). This is used to skip SCALE-only methods when running
// against TrueNAS CORE.
func IsMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*RPCError)
	return ok && e.Code == -32601
}

// IsMethodCallError reports whether err is an RPCError with code -32001
// (middleware method call error). This commonly indicates argument shape
// mismatches across TrueNAS versions.
func IsMethodCallError(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*RPCError)
	return ok && e.Code == -32001
}

// Call performs a single JSON-RPC 2.0 method call against TrueNAS.
//
// It opens a fresh WebSocket connection, authenticates with auth.login_with_api_key,
// sends the method call, reads and unmarshals the result into dest, then closes
// the connection. params may be nil (treated as an empty array).
func (c *Client) Call(ctx context.Context, method string, params []any, dest any) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint, err := c.validatedWSEndpoint()
	if err != nil {
		return fmt.Errorf("truenas ws endpoint validation: %w", err)
	}
	conn, err := dialWS(callCtx, endpoint, c.SkipVerify)
	if err != nil {
		return fmt.Errorf("truenas ws dial %s: %w", endpoint, err)
	}
	defer conn.Close()

	// Propagate context deadline to the underlying connection.
	if deadline, ok := callCtx.Deadline(); ok {
		if setErr := conn.SetWriteDeadline(deadline); setErr != nil {
			return fmt.Errorf("truenas ws set write deadline: %w", setErr)
		}
		if setErr := conn.SetReadDeadline(deadline); setErr != nil {
			return fmt.Errorf("truenas ws set read deadline: %w", setErr)
		}
	}

	// Step 1: authenticate.
	authID := globalRequestID.Add(1)
	authReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      authID,
		Method:  "auth.login_with_api_key",
		Params:  []any{c.APIKey},
	}
	if err := conn.WriteJSON(authReq); err != nil {
		return fmt.Errorf("truenas ws auth send: %w", err)
	}

	var authResp rpcResponse
	if err := conn.ReadJSON(&authResp); err != nil {
		return fmt.Errorf("truenas ws auth read: %w", err)
	}
	if authResp.ID != authID {
		return fmt.Errorf("truenas ws auth: response id mismatch (want %d, got %d)", authID, authResp.ID)
	}
	if authResp.Error != nil {
		return &RPCError{Code: authResp.Error.Code, Message: authResp.Error.Message, Reason: authResp.Error.Reason}
	}

	// Verify that the auth response signals success (result == true).
	var authOK bool
	if err := json.Unmarshal(authResp.Result, &authOK); err != nil {
		return fmt.Errorf("truenas ws auth: unexpected result format: %w", err)
	}
	if !authOK {
		return fmt.Errorf("truenas ws auth: server rejected api key")
	}

	// Step 2: send the actual method call.
	callID := globalRequestID.Add(1)
	callReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      callID,
		Method:  method,
		Params:  normalizeParams(params),
	}
	if err := conn.WriteJSON(callReq); err != nil {
		return fmt.Errorf("truenas ws call send (%s): %w", method, err)
	}

	var callResp rpcResponse
	if err := conn.ReadJSON(&callResp); err != nil {
		return fmt.Errorf("truenas ws call read (%s): %w", method, err)
	}
	if callResp.ID != callID {
		return fmt.Errorf("truenas ws call (%s): response id mismatch (want %d, got %d)", method, callID, callResp.ID)
	}
	if callResp.Error != nil {
		return &RPCError{Code: callResp.Error.Code, Message: callResp.Error.Message, Reason: callResp.Error.Reason}
	}

	// Step 3: unmarshal result into dest when a destination is provided.
	if dest != nil && len(callResp.Result) > 0 {
		if err := json.Unmarshal(callResp.Result, dest); err != nil {
			return fmt.Errorf("truenas ws decode result (%s): %w", method, err)
		}
	}

	return nil
}

// SubscriptionEvent is a normalized event emitted by core.subscribe streams.
type SubscriptionEvent struct {
	Collection     string
	MessageType    string
	EventID        string
	SubscriptionID string
	Fields         map[string]any
	Raw            map[string]any
	ReceivedAt     time.Time
}

// Subscribe opens a core.subscribe stream for the requested collection and
// invokes handler for each received event until ctx is cancelled or the stream
// fails.
func (c *Client) Subscribe(ctx context.Context, collection string, handler func(SubscriptionEvent) error) error {
	collection = strings.TrimSpace(collection)
	if collection == "" {
		return fmt.Errorf("truenas subscription collection is required")
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	connectCtx, connectCancel := context.WithTimeout(ctx, timeout)
	defer connectCancel()

	endpoint, err := c.validatedWSEndpoint()
	if err != nil {
		return fmt.Errorf("truenas ws endpoint validation: %w", err)
	}
	conn, err := dialWS(connectCtx, endpoint, c.SkipVerify)
	if err != nil {
		return fmt.Errorf("truenas ws dial %s: %w", endpoint, err)
	}
	defer conn.Close()

	if deadline, ok := connectCtx.Deadline(); ok {
		if setErr := conn.SetWriteDeadline(deadline); setErr != nil {
			return fmt.Errorf("truenas ws set write deadline: %w", setErr)
		}
		if setErr := conn.SetReadDeadline(deadline); setErr != nil {
			return fmt.Errorf("truenas ws set read deadline: %w", setErr)
		}
	}

	// Step 1: authenticate.
	authID := globalRequestID.Add(1)
	authReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      authID,
		Method:  "auth.login_with_api_key",
		Params:  []any{c.APIKey},
	}
	if err := conn.WriteJSON(authReq); err != nil {
		return fmt.Errorf("truenas ws auth send: %w", err)
	}

	var authResp rpcResponse
	if err := conn.ReadJSON(&authResp); err != nil {
		return fmt.Errorf("truenas ws auth read: %w", err)
	}
	if authResp.ID != authID {
		return fmt.Errorf("truenas ws auth: response id mismatch (want %d, got %d)", authID, authResp.ID)
	}
	if authResp.Error != nil {
		return &RPCError{Code: authResp.Error.Code, Message: authResp.Error.Message, Reason: authResp.Error.Reason}
	}

	var authOK bool
	if err := json.Unmarshal(authResp.Result, &authOK); err != nil {
		return fmt.Errorf("truenas ws auth: unexpected result format: %w", err)
	}
	if !authOK {
		return fmt.Errorf("truenas ws auth: server rejected api key")
	}

	// Step 2: subscribe to collection.
	subscribeID := globalRequestID.Add(1)
	subscribeReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      subscribeID,
		Method:  "core.subscribe",
		Params:  []any{collection},
	}
	if err := conn.WriteJSON(subscribeReq); err != nil {
		return fmt.Errorf("truenas ws subscribe send (%s): %w", collection, err)
	}

	var subscribeResp rpcResponse
	if err := conn.ReadJSON(&subscribeResp); err != nil {
		return fmt.Errorf("truenas ws subscribe read (%s): %w", collection, err)
	}
	if subscribeResp.ID != subscribeID {
		return fmt.Errorf("truenas ws subscribe (%s): response id mismatch (want %d, got %d)", collection, subscribeID, subscribeResp.ID)
	}
	if subscribeResp.Error != nil {
		return &RPCError{Code: subscribeResp.Error.Code, Message: subscribeResp.Error.Message, Reason: subscribeResp.Error.Reason}
	}
	subscriptionID := parseSubscriptionID(subscribeResp.Result)

	// Handshake is complete; remove read deadline so the stream can remain open.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("truenas ws set read deadline: %w", err)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	for {
		raw := map[string]any{}
		if err := conn.ReadJSON(&raw); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("truenas ws subscribe stream (%s): %w", collection, err)
		}

		event, ok := parseSubscriptionEvent(raw, collection, subscriptionID)
		if !ok {
			continue
		}
		if handler == nil {
			continue
		}
		if err := handler(event); err != nil {
			return err
		}
	}
}

// wsEndpoint converts the BaseURL to a WebSocket endpoint.
//
// Scheme mapping:
//   - https -> wss
//   - http  -> ws
//   - wss/ws are passed through unchanged
//
// The path /api/current is always appended.
func (c *Client) wsEndpoint() string {
	raw := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if raw == "" {
		return "wss://localhost/api/current"
	}

	u, err := url.Parse(raw)
	if err != nil {
		// Best-effort fallback: replace scheme prefix manually.
		raw = strings.TrimPrefix(raw, "https://")
		raw = strings.TrimPrefix(raw, "http://")
		return "wss://" + raw + "/api/current"
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
		// already correct
	default:
		u.Scheme = "wss"
	}

	u.Path = "/api/current"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (c *Client) validatedWSEndpoint() (string, error) {
	endpoint := c.wsEndpoint()
	parsed, err := securityruntime.ValidateOutboundURL(endpoint)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// normalizeParams ensures params is never nil so that JSON encoding always
// produces an array literal rather than null.
func normalizeParams(params []any) []any {
	if params == nil {
		return []any{}
	}
	return params
}

func parseSubscriptionID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	return anyToIdentifier(decoded)
}

func parseSubscriptionEvent(raw map[string]any, expectedCollection, subscriptionID string) (SubscriptionEvent, bool) {
	if _, hasResult := raw["result"]; hasResult {
		if raw["method"] == nil && raw["msg"] == nil && raw["collection"] == nil && raw["fields"] == nil {
			return SubscriptionEvent{}, false
		}
	}
	if _, hasError := raw["error"]; hasError {
		if raw["method"] == nil && raw["msg"] == nil && raw["collection"] == nil {
			return SubscriptionEvent{}, false
		}
	}

	collection := strings.TrimSpace(anyToIdentifier(raw["collection"]))
	messageType := strings.TrimSpace(anyToIdentifier(raw["msg"]))
	eventID := strings.TrimSpace(anyToIdentifier(raw["id"]))
	fields := make(map[string]any)

	// TrueNAS may emit JSON-RPC notifications with params arrays.
	if method := strings.TrimSpace(anyToIdentifier(raw["method"])); method != "" {
		collectionFromParams, msgFromParams, fieldsFromParams, subscriptionFromParams := parseNotificationParams(raw["params"])
		if collection == "" {
			collection = collectionFromParams
		}
		if messageType == "" {
			messageType = msgFromParams
		}
		if len(fieldsFromParams) > 0 {
			fields = fieldsFromParams
		}
		if eventID == "" {
			eventID = subscriptionFromParams
		}
	}

	if payload, ok := raw["fields"].(map[string]any); ok {
		fields = payload
	}

	if collection == "" {
		collection = strings.TrimSpace(expectedCollection)
	}
	if messageType == "" {
		messageType = "event"
	}

	// Match on collection first, then subscription ID if the collection is absent.
	if expected := strings.TrimSpace(expectedCollection); expected != "" {
		if collection != "" && !strings.EqualFold(collection, expected) {
			if subscriptionID == "" || !strings.EqualFold(strings.TrimSpace(eventID), strings.TrimSpace(subscriptionID)) {
				return SubscriptionEvent{}, false
			}
		}
	}

	// Ignore method-call responses that are not events.
	if collection == "" && len(fields) == 0 {
		return SubscriptionEvent{}, false
	}

	return SubscriptionEvent{
		Collection:     collection,
		MessageType:    messageType,
		EventID:        eventID,
		SubscriptionID: strings.TrimSpace(subscriptionID),
		Fields:         fields,
		Raw:            raw,
		ReceivedAt:     time.Now().UTC(),
	}, true
}

func parseNotificationParams(value any) (collection, messageType string, fields map[string]any, subscriptionID string) {
	params, ok := value.([]any)
	if !ok || len(params) == 0 {
		return "", "", nil, ""
	}

	subscriptionID = strings.TrimSpace(anyToIdentifier(params[0]))
	if len(params) >= 2 {
		messageType = strings.TrimSpace(anyToIdentifier(params[1]))
	}
	if len(params) >= 3 {
		collection = strings.TrimSpace(anyToIdentifier(params[2]))
	}
	if len(params) >= 4 {
		if payload, ok := params[3].(map[string]any); ok {
			fields = payload
		}
	}

	// Alternate shape: [collection, msg, fields].
	if collection == "" && len(params) >= 1 {
		collection = strings.TrimSpace(anyToIdentifier(params[0]))
	}
	if messageType == "" && len(params) >= 2 {
		messageType = strings.TrimSpace(anyToIdentifier(params[1]))
	}
	if fields == nil && len(params) >= 3 {
		if payload, ok := params[2].(map[string]any); ok {
			fields = payload
		}
	}

	return collection, messageType, fields, subscriptionID
}

func anyToIdentifier(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strings.TrimSpace(strconv.FormatInt(int64(typed), 10))
	case float32:
		return strings.TrimSpace(strconv.FormatInt(int64(typed), 10))
	case int:
		return strings.TrimSpace(strconv.Itoa(typed))
	case int64:
		return strings.TrimSpace(strconv.FormatInt(typed, 10))
	case uint64:
		return strings.TrimSpace(strconv.FormatUint(typed, 10))
	case json.Number:
		return strings.TrimSpace(typed.String())
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}
