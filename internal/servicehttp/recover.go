package servicehttp

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// RecoverMiddleware wraps an HTTP handler to catch panics. On panic it logs
// the stack trace and returns a 500 response. The server process stays alive.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("http: panic recovered on %s %s: %v\n%s", r.Method, r.URL.Path, err, debug.Stack())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))

			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SafeGo launches fn in a goroutine with panic recovery. If fn panics, the
// panic is logged with the goroutine name and fn is restarted after a 1-second
// backoff. The goroutine exits when ctx is cancelled. If wg is non-nil, it is
// used to track the goroutine's lifetime for graceful shutdown.
func SafeGo(ctx context.Context, wg *sync.WaitGroup, name string, fn func(ctx context.Context)) {
	if wg != nil {
		wg.Add(1)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		for {
			if ctx.Err() != nil {
				return
			}
			func() {
				defer func() {
					if err := recover(); err != nil {
						log.Printf("safego[%s]: panic recovered: %v\n%s", name, err, debug.Stack())
					}
				}()
				fn(ctx)
			}()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}
