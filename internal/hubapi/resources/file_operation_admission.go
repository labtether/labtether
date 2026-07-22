package resources

import (
	"errors"
	"net/http"
	"sync"

	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	maxConcurrentInteractiveFileOperations = 32
	maxConcurrentFileTransfers             = 8
	maxConcurrentStagedFileCopies          = 2
)

var (
	errFileOperationCapacity = errors.New("remote file operation capacity reached")

	interactiveFileOperationAdmission = newBoundedAdmission(maxConcurrentInteractiveFileOperations)
	fileTransferAdmission             = newBoundedAdmission(maxConcurrentFileTransfers)
	stagedFileCopyAdmission           = newBoundedAdmission(maxConcurrentStagedFileCopies)
)

// boundedAdmission is a process-wide, non-blocking semaphore. Rejecting once
// capacity is full keeps both active work and waiting goroutines bounded; the
// caller can retry after an existing operation releases its slot.
type boundedAdmission struct {
	slots chan struct{}
}

func newBoundedAdmission(limit int) *boundedAdmission {
	if limit <= 0 {
		panic("bounded admission limit must be positive")
	}
	return &boundedAdmission{slots: make(chan struct{}, limit)}
}

func (a *boundedAdmission) tryAcquire() (release func(), ok bool) {
	select {
	case a.slots <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-a.slots })
		}, true
	default:
		return nil, false
	}
}

func writeFileOperationCapacityError(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	servicehttp.WriteError(w, http.StatusTooManyRequests, "remote file operation capacity reached; retry shortly")
}
