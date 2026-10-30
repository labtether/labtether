package shared

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/labtether/labtether/internal/terminal"
)

// SessionStreamRegistry binds pending and active browser requests to a logical
// session. Deleting its record and rejecting new stream work share one gate.
// Entries live only while requests or registry operations retain them.
type SessionStreamRegistry struct {
	mu         sync.Mutex
	entries    map[string]*sessionStreamEntry
	principals map[string]map[*sessionStreamRequest]struct{} // protected by mu
}

type sessionStreamEntry struct {
	mu       sync.Mutex
	refs     int // protected by the registry mutex
	requests map[*sessionStreamRequest]struct{}
}

type sessionStreamRequest struct {
	parent     *sessionStreamRequest
	entry      *sessionStreamEntry
	cancel     context.CancelFunc
	closed     bool // protected by entry.mu
	conn       net.Conn
	principals []string // credentials behind the stream; see RevokePrincipals
}

type sessionStreamContextKey struct{}

func (r *SessionStreamRegistry) retain(id string) (*sessionStreamEntry, func()) {
	r.mu.Lock()
	if r.entries == nil {
		r.entries = make(map[string]*sessionStreamEntry)
	}
	entry := r.entries[id]
	if entry == nil {
		entry = &sessionStreamEntry{requests: make(map[*sessionStreamRequest]struct{})}
		r.entries[id] = entry
	}
	entry.refs++
	r.mu.Unlock()
	return entry, func() {
		r.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(r.entries, id)
		}
		r.mu.Unlock()
	}
}

// Open rechecks the persisted record while deletion for this session is held.
// The returned writer registers the hijacked socket, including a delayed upgrade
// that began before DELETE. Cleanup must run when the request handler returns.
func (r *SessionStreamRegistry) Open(w http.ResponseWriter, request *http.Request, id string, exists func() (bool, error)) (http.ResponseWriter, *http.Request, func(), error) {
	entry, release := r.retain(id)
	entry.mu.Lock()
	present, err := exists()
	if err == nil && !present {
		err = terminal.ErrSessionNotFound
	}
	if err == nil {
		err = request.Context().Err()
	}
	if err != nil {
		entry.mu.Unlock()
		release()
		return w, request, nil, err
	}
	ctx, cancel := context.WithCancel(request.Context())
	parent, _ := request.Context().Value(sessionStreamContextKey{}).(*sessionStreamRequest)
	owner := &sessionStreamRequest{entry: entry, cancel: cancel, parent: parent, principals: StreamPrincipalKeys(request.Context())}
	entry.requests[owner] = struct{}{}
	// Index while the entry is held so a revocation cannot miss this stream.
	r.indexPrincipals(owner)
	entry.mu.Unlock()
	ctx = context.WithValue(ctx, sessionStreamContextKey{}, owner)
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			entry.mu.Lock()
			delete(entry.requests, owner)
			owner.closed = true
			conn := owner.conn
			entry.mu.Unlock()
			r.unindexPrincipals(owner)
			cancel()
			if conn != nil {
				_ = conn.Close()
			}
			release()
		})
	}
	return sessionStreamResponse{ResponseWriter: w, owner: owner}, request.WithContext(ctx), cleanup, nil
}

// Delete preserves live streams if storage deletion fails. On success it retires
// all requests before releasing the gate, then cancels/closes outside every lock.
func (r *SessionStreamRegistry) Delete(id string, remove func() error) error {
	entry, release := r.retain(id)
	defer release()
	entry.mu.Lock()
	if err := remove(); err != nil {
		entry.mu.Unlock()
		return err
	}
	owners := make([]*sessionStreamRequest, 0, len(entry.requests))
	conns := make([]net.Conn, 0, len(entry.requests))
	for owner := range entry.requests {
		owner.closed = true
		owners = append(owners, owner)
		if owner.conn != nil {
			conns = append(conns, owner.conn)
		}
	}
	entry.mu.Unlock()
	for _, owner := range owners {
		owner.cancel()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	return nil
}

// RunIfExists serializes nonstream operations such as creating a saved-shell
// attachment with deletion of their parent record. The action must not reenter
// this same key; nested logical-session keys follow the persistent-session key.
func (r *SessionStreamRegistry) RunIfExists(id string, exists func() (bool, error), action func()) error {
	entry, release := r.retain(id)
	defer release()
	entry.mu.Lock()
	defer entry.mu.Unlock()
	present, err := exists()
	if err != nil {
		return err
	}
	if !present {
		return terminal.ErrSessionNotFound
	}
	action()
	return nil
}

// RunIfSessionStreamOpen guards stream publication and start sends. Nested
// owners lock persistent session, logical session, then any agent lifecycle.
// Callbacks must not reenter these gates. A start send may delay DELETE up to
// the existing agent write bound. Unregistered contexts retain direct callers.
func RunIfSessionStreamOpen(ctx context.Context, action func()) bool {
	owner, _ := ctx.Value(sessionStreamContextKey{}).(*sessionStreamRequest)
	return runSessionStreamOwner(ctx, owner, action)
}

func runSessionStreamOwner(ctx context.Context, owner *sessionStreamRequest, action func()) bool {
	if owner == nil {
		action()
		return true
	}
	allowed := false
	runSessionStreamOwner(ctx, owner.parent, func() {
		owner.entry.mu.Lock()
		defer owner.entry.mu.Unlock()
		if !owner.closed && ctx.Err() == nil {
			action()
			allowed = true
		}
	})
	return allowed
}

type sessionStreamResponse struct {
	http.ResponseWriter
	owner *sessionStreamRequest
}

func (w sessionStreamResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w sessionStreamResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return conn, rw, err
	}
	w.owner.entry.mu.Lock()
	if w.owner.closed {
		w.owner.entry.mu.Unlock()
		_ = conn.Close()
		return nil, nil, net.ErrClosed
	}
	w.owner.conn = conn
	w.owner.entry.mu.Unlock()
	return conn, rw, nil
}
