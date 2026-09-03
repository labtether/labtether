package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

func TestBuildConnectionConfigPinsFirstSFTPHostKey(t *testing.T) {
	connection := &persistence.FileConnection{
		ID:          "fconn-host-key",
		Protocol:    "sftp",
		Host:        "sftp.example.invalid",
		InitialPath: "/",
		ExtraConfig: map[string]any{},
	}
	store := newTestTransferFileConnectionStore(connection)
	deps := &Deps{FileConnectionStore: store}

	config, err := deps.buildConnectionConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	if config.PersistHostKey == nil {
		t.Fatalf("saved SFTP config did not install a durable host key policy: %#v", config)
	}
	const presentedKey = "ssh-ed25519 synthetic-public-host-key"
	if err := config.PersistHostKey(context.Background(), presentedKey); err != nil {
		t.Fatalf("pin first host key: %v", err)
	}
	stored, err := store.GetFileConnection(context.Background(), connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ExtraConfig["host_key"] != presentedKey {
		t.Fatalf("first host key was not saved: %#v", stored.ExtraConfig)
	}
	if err := config.PersistHostKey(context.Background(), "ssh-ed25519 different-public-host-key"); err == nil || !strings.Contains(err.Error(), "host key changed") {
		t.Fatalf("different concurrent host key was not blocked safely: %v", err)
	}
}

func TestBuildConnectionConfigRejectsEndpointEditDuringSFTPHandshake(t *testing.T) {
	connection := &persistence.FileConnection{
		ID:          "fconn-endpoint-edit",
		Protocol:    "sftp",
		Host:        "before.example.invalid",
		InitialPath: "/",
	}
	store := newTestTransferFileConnectionStore(connection)
	deps := &Deps{FileConnectionStore: store}
	config, err := deps.buildConnectionConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	store.connections[connection.ID].Host = "after.example.invalid"
	if err := config.PersistHostKey(context.Background(), "ssh-ed25519 synthetic-public-host-key"); err == nil || !strings.Contains(err.Error(), "connection changed") {
		t.Fatalf("stale endpoint pin was not blocked safely: %v", err)
	}
}

func TestBuildConnectionConfigHidesSFTPHostKeyStoreFailure(t *testing.T) {
	connection := &persistence.FileConnection{
		ID:          "fconn-store-error",
		Protocol:    "sftp",
		Host:        "sftp.example.invalid",
		InitialPath: "/",
	}
	store := newTestTransferFileConnectionStore(connection)
	store.pinErr = errors.New("database detail that must stay private")
	deps := &Deps{FileConnectionStore: store}
	config, err := deps.buildConnectionConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	err = config.PersistHostKey(context.Background(), "ssh-ed25519 synthetic-public-host-key")
	if err == nil || err.Error() != "failed to save trusted SFTP host key" {
		t.Fatalf("unsafe persistence error: %v", err)
	}
}
