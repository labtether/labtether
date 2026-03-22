package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/ssh"
)

func mustPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer.PublicKey()
}

func TestBuildCollectorSSHHostKeyCallbackAcceptsExpectedFingerprint(t *testing.T) {
	publicKey := mustPublicKey(t)
	expected := ssh.FingerprintSHA256(publicKey)

	callback, insecure, err := buildCollectorSSHHostKeyCallback(map[string]any{
		"strict_host_key": true,
		"host_key":        expected,
	})
	if err != nil {
		t.Fatalf("build callback: %v", err)
	}
	if insecure {
		t.Fatalf("expected strict callback, got insecure")
	}
	if err := callback("example", nil, publicKey); err != nil {
		t.Fatalf("expected host key to match, got error: %v", err)
	}
}

func TestBuildCollectorSSHHostKeyCallbackRejectsUnexpectedKey(t *testing.T) {
	publicKey := mustPublicKey(t)
	expected := ssh.FingerprintSHA256(publicKey)

	callback, insecure, err := buildCollectorSSHHostKeyCallback(map[string]any{
		"strict_host_key": true,
		"host_key":        expected,
	})
	if err != nil {
		t.Fatalf("build callback: %v", err)
	}
	if insecure {
		t.Fatalf("expected strict callback, got insecure")
	}

	unexpected := mustPublicKey(t)
	if err := callback("example", nil, unexpected); err == nil {
		t.Fatalf("expected mismatch error for unexpected host key")
	}
}

func TestBuildCollectorSSHHostKeyCallbackSupportsExplicitInsecureMode(t *testing.T) {
	callback, insecure, err := buildCollectorSSHHostKeyCallback(map[string]any{
		"strict_host_key": false,
	})
	if err != nil {
		t.Fatalf("build callback: %v", err)
	}
	if callback == nil {
		t.Fatalf("expected callback")
	}
	if !insecure {
		t.Fatalf("expected insecure flag to be true")
	}
}
