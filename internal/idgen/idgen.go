package idgen

import (
	"fmt"
	"sync/atomic"
	"time"
)

var counter atomic.Uint64

// New returns a sortable, process-unique identifier with the provided prefix.
func New(prefix string) string {
	n := counter.Add(1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UTC().UnixNano(), n)
}
