package servicehttp

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/certmgr"
	"net/http"
	"sync"
	"testing"
	"time"
)

type shutdownServerStub struct {
	shutdownErr error
	closeErr    error
	closeCalls  int
}

func (s *shutdownServerStub) Shutdown(context.Context) error {
	return s.shutdownErr
}

func (s *shutdownServerStub) Close() error {
	s.closeCalls++
	return s.closeErr
}

func TestDrainHTTPServerForcesCloseAndPreservesTimeoutClassification(t *testing.T) {
	server := &shutdownServerStub{shutdownErr: context.DeadlineExceeded}

	err := drainHTTPServer("test", server, time.Second)
	if !errors.Is(err, ErrHTTPDrainIncomplete) {
		t.Fatalf("drain error = %v, want ErrHTTPDrainIncomplete", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain error = %v, want DeadlineExceeded", err)
	}
	if server.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", server.closeCalls)
	}
	if got := classifyHTTPDrainFailure(err); got != "timeout" {
		t.Fatalf("classification = %q, want timeout", got)
	}
}

func TestRunCancelsActiveHandlerContextBeforeGracefulDrain(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "2")
	port := freeTCPPort(t)
	started := make(chan struct{})
	handlerStopped := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:        "shutdown-context-test",
			BindAddress: "127.0.0.1",
			Port:        fmt.Sprintf("%d", port),
			ExtraHandlers: map[string]http.HandlerFunc{
				"/slow": func(w http.ResponseWriter, r *http.Request) {
					close(started)
					<-r.Context().Done()
					close(handlerStopped)
				},
			},
		})
	}()
	waitForTCPServer(t, port)

	requestDone := make(chan struct{})
	go func() {
		resp, _ := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow handler did not start")
	}
	cancel()

	select {
	case <-handlerStopped:
	case <-time.After(time.Second):
		t.Fatal("active handler context was not canceled")
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error after context-aware handler drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context-aware handler stopped")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}
}

func TestRunPropagatesShutdownTimeoutAfterForcedClose(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "1")
	port := freeTCPPort(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:        "shutdown-timeout-test",
			BindAddress: "127.0.0.1",
			Port:        fmt.Sprintf("%d", port),
			ExtraHandlers: map[string]http.HandlerFunc{
				"/blocked": func(http.ResponseWriter, *http.Request) {
					close(started)
					<-release
				},
			},
		})
	}()
	waitForTCPServer(t, port)

	requestDone := make(chan struct{})
	go func() {
		resp, _ := http.Get(fmt.Sprintf("http://127.0.0.1:%d/blocked", port))
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocked handler did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrHTTPDrainIncomplete) {
			t.Fatalf("Run error = %v, want ErrHTTPDrainIncomplete", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run error = %v, want DeadlineExceeded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after forced close")
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("forced-close request did not finish")
	}
}

func TestRunWaitsForActiveMainHandlerShutdown(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "2")
	port := freeTCPPort(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:        "shutdown-main-test",
			BindAddress: "127.0.0.1",
			Port:        fmt.Sprintf("%d", port),
			ExtraHandlers: map[string]http.HandlerFunc{
				"/slow": func(w http.ResponseWriter, _ *http.Request) {
					close(started)
					<-release
					w.WriteHeader(http.StatusNoContent)
				},
			},
		})
	}()
	waitForTCPServer(t, port)

	requestDone := make(chan error, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if resp != nil {
			_ = resp.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow main handler did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before active main handler drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-requestDone; err != nil {
		t.Fatalf("slow main request failed: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error after main drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after main handler drained")
	}
}

func TestRunWaitsForActiveRedirectHandlerShutdown(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "2")
	mainPort := freeTCPPort(t)
	redirectPort := freeTCPPort(t)
	certs, err := certmgr.Provision(t.TempDir(), "127.0.0.1")
	if err != nil {
		t.Fatalf("provision test certificate: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:             "shutdown-redirect-test",
			BindAddress:      "127.0.0.1",
			Port:             fmt.Sprintf("%d", mainPort),
			TLSCertFile:      certs.ServerCertPath,
			TLSKeyFile:       certs.ServerKeyPath,
			RedirectHTTPPort: fmt.Sprintf("%d", redirectPort),
			HTTPSPort:        mainPort,
			ExtraHandlers: map[string]http.HandlerFunc{
				"/api/v1/tls/info": func(w http.ResponseWriter, _ *http.Request) {
					close(started)
					<-release
					w.WriteHeader(http.StatusNoContent)
				},
			},
		})
	}()
	waitForTCPServer(t, mainPort)
	waitForTCPServer(t, redirectPort)

	requestDone := make(chan error, 1)
	go func() {
		resp, requestErr := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/tls/info", redirectPort))
		if resp != nil {
			_ = resp.Body.Close()
		}
		requestDone <- requestErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow redirect handler did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before active redirect handler drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-requestDone; err != nil {
		t.Fatalf("slow redirect request failed: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error after redirect drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after redirect handler drained")
	}
}
