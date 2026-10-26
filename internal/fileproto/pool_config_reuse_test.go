package fileproto

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPoolReusesConnectionWithFreshConfigCallbacks(t *testing.T) {
	pool := NewPool()
	defer pool.Close()
	mock := &mockRemoteFS{}
	pool.newFS = func(string) (RemoteFS, error) { return mock, nil }
	validations := 0
	for range 3 {
		config := ConnectionConfig{
			ConnectionID: "saved", Protocol: "sftp", Host: "192.0.2.1", Secret: "test-secret",
			ValidateCurrent: func(context.Context) error { validations++; return nil },
			PersistHostKey:  func(context.Context, string) error { return nil },
		}
		fs, err := pool.Get(context.Background(), "saved", config)
		if err != nil || fs != mock {
			t.Fatalf("reuse failed: %v", err)
		}
	}
	if validations != 3 || mock.connectCalls != 1 || mock.closeCalls != 0 {
		t.Fatalf("expected each validation and one connect: validations=%d connects=%d closes=%d", validations, mock.connectCalls, mock.closeCalls)
	}

	invalid := errors.New("saved credential changed")
	_, err := pool.Get(context.Background(), "saved", ConnectionConfig{
		ConnectionID: "saved", Protocol: "sftp", Host: "192.0.2.1", Secret: "test-secret",
		ValidateCurrent: func(context.Context) error { return invalid },
	})
	if !errors.Is(err, invalid) || mock.connectCalls != 1 {
		t.Fatalf("current-config validation was bypassed: %v", err)
	}
}

func TestPoolCallbackConfigStillReconnectsAfterCredentialOrEndpointChange(t *testing.T) {
	for _, change := range []string{"secret", "host", "port", "TLS", "host key"} {
		t.Run(change, func(t *testing.T) {
			pool := NewPool()
			defer pool.Close()
			var mocks []*mockRemoteFS
			pool.newFS = func(string) (RemoteFS, error) {
				mock := &mockRemoteFS{}
				mocks = append(mocks, mock)
				return mock, nil
			}
			config := ConnectionConfig{
				Protocol: "sftp", Host: "192.0.2.1", Port: 22, Secret: "old-test-secret",
				ExtraConfig:     map[string]any{"host_key": "original"},
				ValidateCurrent: func(context.Context) error { return nil },
				PersistHostKey:  func(context.Context, string) error { return nil },
			}
			if _, err := pool.Get(context.Background(), "saved", config); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "secret":
				config.Secret = "new-test-secret"
			case "host":
				config.Host = "192.0.2.2"
			case "port":
				config.Port = 2222
			case "TLS":
				config.ExtraConfig["webdav_tls"] = false
			case "host key":
				config.ExtraConfig["host_key"] = "replacement"
			}
			if _, err := pool.Get(context.Background(), "saved", config); err != nil {
				t.Fatal(err)
			}
			if len(mocks) != 2 || mocks[0].closeCalls != 1 || mocks[1].connectCalls != 1 {
				t.Fatal("changed connection reused old transport or credentials")
			}
		})
	}
}

func TestPoolConcurrentEquivalentCallbackConfigsReuseWinningSession(t *testing.T) {
	pool := NewPool()
	defer pool.Close()
	var connected atomic.Int32
	var validations atomic.Int32
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	pool.newFS = func(string) (RemoteFS, error) {
		connected.Add(1)
		ready <- struct{}{}
		return &mockRemoteFS{connectDone: release}, nil
	}
	var workers sync.WaitGroup
	results := make(chan RemoteFS, 2)
	errorsSeen := make(chan error, 2)
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			fs, err := pool.Get(context.Background(), "saved", ConnectionConfig{
				Protocol: "sftp", Host: "192.0.2.1",
				ValidateCurrent: func(context.Context) error { validations.Add(1); return nil },
				PersistHostKey:  func(context.Context, string) error { return nil },
			})
			results <- fs
			errorsSeen <- err
		}()
	}
	<-ready
	<-ready
	close(release)
	workers.Wait()
	for range 2 {
		if err := <-errorsSeen; err != nil {
			t.Fatalf("equivalent racing config rejected: %v", err)
		}
	}
	if first, second := <-results, <-results; first == nil || first != second {
		t.Fatal("racing equivalent configs did not share winning session")
	}
	if validations.Load() != 2 || connected.Load() != 2 {
		t.Fatal("test did not exercise both concurrent connection attempts")
	}
}
