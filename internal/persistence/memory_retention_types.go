package persistence

import (
	"sync"

	"github.com/labtether/labtether/internal/retention"
)

type MemoryRetentionStore struct {
	mu       sync.RWMutex
	settings retention.Settings
}
