package fileproto

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPoolConnectionMutationWaitsForConnectAndRejectsStaleConfig(t *testing.T) {
	pool := NewPool()
	defer pool.Close()
	mock := &mockRemoteFS{
		connectStart: make(chan struct{}),
		connectNext:  make(chan struct{}),
	}
	pool.newFS = func(string) (RemoteFS, error) { return mock, nil }

	var revision int
	revision = 1
	oldConfig := ConnectionConfig{
		ConnectionID: "conn-race",
		Protocol:     "sftp",
		ValidateCurrent: func(context.Context) error {
			if revision != 1 {
				return errors.New("saved connection changed")
			}
			return nil
		},
	}
	getDone := make(chan error, 1)
	go func() {
		_, err := pool.Get(context.Background(), "transfer-race", oldConfig)
		getDone <- err
	}()
	<-mock.connectStart

	mutationStarted := make(chan struct{})
	mutationDone := make(chan struct{})
	go func() {
		finish := pool.BeginConnectionMutation("conn-race")
		close(mutationStarted)
		revision = 2
		finish()
		close(mutationDone)
	}()
	select {
	case <-mutationStarted:
		t.Fatal("connection mutation started before in-flight connect finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(mock.connectNext)
	if err := <-getDone; err != nil {
		t.Fatalf("in-flight connection failed before mutation: %v", err)
	}
	<-mutationDone
	pool.mu.Lock()
	sessionCount := len(pool.sessions)
	pool.mu.Unlock()
	if sessionCount != 0 {
		t.Fatalf("mutation left %d stale pooled sessions", sessionCount)
	}
	if _, err := pool.Get(context.Background(), "transfer-race", oldConfig); err == nil || !strings.Contains(err.Error(), "saved connection changed") {
		t.Fatalf("stale config was allowed after mutation: %v", err)
	}
}

func TestPool_Get_ReplacesSessionWhenConfigChanges(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	oldMock := &mockRemoteFS{}
	newMock := &mockRemoteFS{}
	oldConfig := ConnectionConfig{
		Protocol:    "ftp",
		InitialPath: "/",
		ExtraConfig: map[string]any{"ftp_tls": false},
	}
	newConfig := ConnectionConfig{
		Protocol:    "ftp",
		InitialPath: "/",
		ExtraConfig: map[string]any{"ftp_tls": true},
	}
	tp.injectMock("conn-1", oldMock, oldConfig)
	tp.Pool.newFS = func(string) (RemoteFS, error) { return newMock, nil }

	fs, err := tp.Pool.Get(context.Background(), "conn-1", newConfig)
	if err != nil {
		t.Fatalf("get changed config: %v", err)
	}
	if fs != newMock {
		t.Fatal("expected a new session for the changed config")
	}
	oldMock.mu.Lock()
	oldClosed := oldMock.closed
	oldMock.mu.Unlock()
	if !oldClosed {
		t.Fatal("expected the old-config session to be closed")
	}
}

func TestPool_Get_DoesNotInsertSessionAfterRemove(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	mock := &mockRemoteFS{connectStart: started, connectDone: release}
	tp.Pool.newFS = func(string) (RemoteFS, error) { return mock, nil }
	config := ConnectionConfig{Protocol: "sftp", InitialPath: "/"}

	result := make(chan error, 1)
	go func() {
		_, err := tp.Pool.Get(context.Background(), "conn-1", config)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connect did not start")
	}
	tp.Pool.Remove("conn-1")
	close(release)

	select {
	case err := <-result:
		if !errors.Is(err, ErrConnectionConfigChanged) {
			t.Fatalf("expected config-changed error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Get did not return")
	}
	if n := tp.sessionCount(); n != 0 {
		t.Fatalf("expected no stale cached session, got %d", n)
	}
	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	if !closed {
		t.Fatal("expected stale in-flight session to be closed")
	}
}

func TestPool_Close_DoesNotLeakInflightSession(t *testing.T) {
	tp := newTestPool()
	started := make(chan struct{})
	release := make(chan struct{})
	mock := &mockRemoteFS{connectStart: started, connectDone: release}
	tp.Pool.newFS = func(string) (RemoteFS, error) { return mock, nil }

	result := make(chan error, 1)
	go func() {
		_, err := tp.Pool.Get(
			context.Background(),
			"conn-1",
			ConnectionConfig{Protocol: "sftp", InitialPath: "/"},
		)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connect did not start")
	}
	tp.Close()
	close(release)

	select {
	case err := <-result:
		if !errors.Is(err, ErrPoolClosed) {
			t.Fatalf("expected pool-closed error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Get did not return")
	}
	if n := tp.sessionCount(); n != 0 {
		t.Fatalf("expected no cached sessions after close, got %d", n)
	}
	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	if !closed {
		t.Fatal("expected in-flight session to be closed after pool shutdown")
	}
}

func TestPool_GetAtGeneration_RejectsConfigLoadedBeforeRemove(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	expectedGeneration := tp.Pool.AcquireGeneration("conn-1")
	defer tp.Pool.ReleaseGeneration("conn-1")
	tp.Pool.Remove("conn-1")
	factoryCalled := false
	tp.Pool.newFS = func(string) (RemoteFS, error) {
		factoryCalled = true
		return &mockRemoteFS{}, nil
	}

	_, err := tp.Pool.GetAtGeneration(
		context.Background(),
		"conn-1",
		ConnectionConfig{Protocol: "sftp", InitialPath: "/"},
		expectedGeneration,
	)
	if !errors.Is(err, ErrConnectionConfigChanged) {
		t.Fatalf("expected config-changed error, got %v", err)
	}
	if factoryCalled {
		t.Fatal("stale config reached the connection factory")
	}
}

func TestPool_Get_DoesNotReturnSessionRemovedDuringHealthCheck(t *testing.T) {
	tp := newTestPool()
	defer tp.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	mock := &mockRemoteFS{listStart: started, listDone: release}
	config := ConnectionConfig{Protocol: "sftp", InitialPath: "/"}
	tp.injectMock("conn-1", mock, config)

	result := make(chan error, 1)
	go func() {
		_, err := tp.Pool.Get(context.Background(), "conn-1", config)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("health check did not start")
	}
	tp.Pool.Remove("conn-1")
	close(release)

	select {
	case err := <-result:
		if !errors.Is(err, ErrConnectionConfigChanged) {
			t.Fatalf("expected config-changed error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Get did not return")
	}
}
