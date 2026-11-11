package terminal

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// checkedHostKeyTTL is how long a key a check saw may be trusted.
const checkedHostKeyTTL = 5 * time.Minute

// maxCheckedHostKeys bounds the remembered checks; the oldest go first.
const maxCheckedHostKeys = 1024

// checkedHostKeys remembers, in memory, the key each check saw for one person,
// host, port and bookmark, so trust can pin only a key the host presented to
// the Hub for that person shortly before.
type checkedHostKeys struct {
	mu      sync.Mutex
	entries map[string]checkedHostKey
}

type checkedHostKey struct {
	hostKey string
	seenAt  time.Time
}

func checkedHostKeyID(actorID, target, bookmarkID string) string {
	return strings.Join([]string{actorID, target, bookmarkID}, "\x00")
}

func (d *Deps) checkedKeys() *checkedHostKeys {
	d.checkedHostKeysOnce.Do(func() {
		d.checkedHostKeys = &checkedHostKeys{entries: make(map[string]checkedHostKey)}
	})
	return d.checkedHostKeys
}

func (c *checkedHostKeys) remember(id, hostKey string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if now.Sub(entry.seenAt) > checkedHostKeyTTL {
			delete(c.entries, key)
		}
	}
	if len(c.entries) >= maxCheckedHostKeys {
		oldestKey, oldest := "", now
		for key, entry := range c.entries {
			if entry.seenAt.Before(oldest) {
				oldestKey, oldest = key, entry.seenAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[id] = checkedHostKey{hostKey: hostKey, seenAt: now}
}

// take reports whether key is the one the last check saw for id within the
// TTL, and forgets that check when it is.
func (c *checkedHostKeys) take(id string, key ssh.PublicKey, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[id]
	if !ok || now.Sub(entry.seenAt) > checkedHostKeyTTL || !sameHostKey(entry.hostKey, key) {
		return false
	}
	delete(c.entries, id)
	return true
}
