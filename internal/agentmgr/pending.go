package agentmgr

import (
	"strings"
	"sync"
	"sync/atomic"
)

// Pending holds the requests that wait for one reply from an agent. Request
// registers a waiter before it sends and removes it when it returns; a router
// handler calls Deliver with the decoded reply. The zero value is ready to use
// and has no admission limit.
//
// Deliver runs inside the agent's read loop, under that device's dispatch
// lock, so it never blocks: each waiter takes at most one reply, and a late,
// duplicate or refused reply is dropped.
type Pending[T any] struct {
	mu      sync.Mutex
	waiters map[string]*waiter[T]
	// reserved holds keys kept only for uniqueness. They are a separate
	// namespace: no reply can reach them, and they never clash with a wire ID.
	reserved map[string]*waiter[T]
	limit    int
	active   int
}

// NewPending returns a registry that admits at most limit requests at once.
// A limit of zero or less means no limit.
func NewPending[T any](limit int) *Pending[T] {
	if limit < 0 {
		limit = 0
	}
	return &Pending[T]{limit: limit}
}

type waiter[T any] struct {
	device    string
	conn      *AgentConn
	accept    func(T) bool
	ch        chan T
	delivered atomic.Bool
}

// KeyRule says which ID in a reply finds its waiter.
type KeyRule int

const (
	// KeyPayload uses the request ID carried in the reply payload and ignores
	// the envelope ID.
	KeyPayload KeyRule = iota
	// KeyEnvelope uses the envelope ID (Message.ID) and ignores the payload.
	KeyEnvelope
	// KeyPayloadEnvelopeIfSet uses the payload request ID; a non-empty envelope
	// ID must equal it.
	KeyPayloadEnvelopeIfSet
	// KeyBoth needs a non-empty envelope ID equal to the payload request ID.
	KeyBoth
)

// DeviceRule says how the replying connection's device ID is compared with
// the device the waiter expects. A waiter that expects no device ("") skips
// the comparison.
type DeviceRule int

const (
	// DeviceAny does no device check; a reply with no connection is accepted.
	DeviceAny DeviceRule = iota
	// DeviceFold trims both IDs and ignores letter case.
	DeviceFold
	// DeviceExact compares the IDs byte for byte, without trimming.
	DeviceExact
	// DeviceExactTrim trims the connection's ID and compares exactly.
	DeviceExactTrim
)

// Match is the rule set a router handler applies to every reply it delivers.
// Registries shared by several reply types may use a different Match per type.
type Match struct {
	Key KeyRule
	// TrimKey trims spaces around the key, and around both IDs that KeyBoth or
	// KeyPayloadEnvelopeIfSet compare.
	TrimKey bool
	Device  DeviceRule
	// AcceptNoConn lets a reply with no connection skip the device check.
	// Without it such a reply is dropped, unless Device is DeviceAny.
	AcceptNoConn bool
}

// Reply is one decoded agent reply offered to a registry.
type Reply[T any] struct {
	// EnvelopeID is Message.ID.
	EnvelopeID string
	// PayloadID is the request ID inside the payload, if the type has one.
	PayloadID string
	// Conn is the connection the reply came in on; nil for in-process callers.
	Conn  *AgentConn
	Value T
}

// Waiting describes one registered key, for tests and diagnostics.
type Waiting struct {
	Key    string
	Device string
}

func (m Match) key(envelopeID, payloadID string) (string, bool) {
	if m.TrimKey {
		envelopeID = strings.TrimSpace(envelopeID)
		payloadID = strings.TrimSpace(payloadID)
	}
	switch m.Key {
	case KeyPayload:
		return payloadID, payloadID != ""
	case KeyEnvelope:
		return envelopeID, envelopeID != ""
	case KeyPayloadEnvelopeIfSet:
		if envelopeID != "" && envelopeID != payloadID {
			return "", false
		}
		return payloadID, payloadID != ""
	case KeyBoth:
		if envelopeID == "" || envelopeID != payloadID {
			return "", false
		}
		return payloadID, true
	default:
		return "", false
	}
}

func (m Match) deviceAllowed(expected string, conn *AgentConn) bool {
	if m.Device == DeviceAny {
		return true
	}
	if conn == nil {
		return m.AcceptNoConn
	}
	if expected == "" {
		return true
	}
	switch m.Device {
	case DeviceFold:
		return strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(conn.AssetID))
	case DeviceExact:
		return conn.AssetID == expected
	case DeviceExactTrim:
		return strings.TrimSpace(conn.AssetID) == expected
	default:
		return false
	}
}

// Deliver hands reply to the waiter its key names when every rule passes. It
// reports whether the reply was delivered. It never blocks: a waiter takes at
// most one reply, and a refused reply leaves the waiter registered.
func (p *Pending[T]) Deliver(match Match, reply Reply[T]) bool {
	if p == nil {
		return false
	}
	key, ok := match.key(reply.EnvelopeID, reply.PayloadID)
	if !ok {
		return false
	}
	p.mu.Lock()
	w := p.waiters[key]
	p.mu.Unlock()
	if w == nil {
		return false
	}
	if !match.deviceAllowed(w.device, reply.Conn) {
		return false
	}
	if w.conn != nil && w.conn != reply.Conn {
		return false
	}
	if w.accept != nil && !w.accept(reply.Value) {
		return false
	}
	if !w.delivered.CompareAndSwap(false, true) {
		return false
	}
	// The channel holds one value and only the CAS winner sends, so this
	// never blocks even when the request has already returned.
	w.ch <- reply.Value
	return true
}

// Len returns the number of registered keys, reply keys and reserved keys.
func (p *Pending[T]) Len() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.waiters) + len(p.reserved)
}

// Load reports whether key has a waiter.
func (p *Pending[T]) Load(key string) (Waiting, bool) {
	if p == nil {
		return Waiting{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	w, ok := p.waiters[key]
	if !ok {
		return Waiting{}, false
	}
	return Waiting{Key: key, Device: w.device}, true
}

// register admits one request and takes its reply key and reserved keys, or
// takes none of them.
func (p *Pending[T]) register(key string, reserve []string, w *waiter[T]) RequestErrorKind {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.limit > 0 && p.active >= p.limit {
		return RequestBusy
	}
	if _, taken := p.waiters[key]; taken {
		return RequestDuplicate
	}
	for i, extra := range reserve {
		if _, taken := p.reserved[extra]; taken {
			return RequestDuplicate
		}
		for _, earlier := range reserve[:i] {
			if earlier == extra {
				return RequestDuplicate
			}
		}
	}
	if p.waiters == nil {
		p.waiters = make(map[string]*waiter[T])
	}
	p.waiters[key] = w
	if len(reserve) > 0 && p.reserved == nil {
		p.reserved = make(map[string]*waiter[T])
	}
	for _, extra := range reserve {
		p.reserved[extra] = w
	}
	p.active++
	return ""
}

// unregister removes the keys that still name w and releases its admission.
func (p *Pending[T]) unregister(key string, reserve []string, w *waiter[T]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.waiters[key] == w {
		delete(p.waiters, key)
	}
	for _, extra := range reserve {
		if p.reserved[extra] == w {
			delete(p.reserved, extra)
		}
	}
	p.active--
}
