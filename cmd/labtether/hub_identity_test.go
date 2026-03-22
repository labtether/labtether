package main

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateHubSSHKeypairEd25519(t *testing.T) {
	privPEM, pubKeyStr, err := generateHubSSHKeypair("ed25519")
	if err != nil {
		t.Fatalf("generateHubSSHKeypair(\"ed25519\") returned error: %v", err)
	}

	if len(privPEM) == 0 {
		t.Fatal("expected non-empty private key PEM")
	}

	signer, err := ssh.ParsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("ssh.ParsePrivateKey failed: %v", err)
	}

	// Verify the round-tripped public key matches what was returned.
	roundTripped := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	if roundTripped != pubKeyStr {
		t.Errorf("public key mismatch after round-trip\ngot:  %s\nwant: %s", roundTripped, pubKeyStr)
	}

	if !strings.HasPrefix(pubKeyStr, "ssh-ed25519 ") {
		t.Errorf("expected public key to start with \"ssh-ed25519 \", got: %s", pubKeyStr)
	}
}

func TestGenerateHubSSHKeypairRSA(t *testing.T) {
	privPEM, pubKeyStr, err := generateHubSSHKeypair("rsa")
	if err != nil {
		t.Fatalf("generateHubSSHKeypair(\"rsa\") returned error: %v", err)
	}

	if len(privPEM) == 0 {
		t.Fatal("expected non-empty private key PEM")
	}

	signer, err := ssh.ParsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("ssh.ParsePrivateKey failed: %v", err)
	}

	// Verify the round-tripped public key matches what was returned.
	roundTripped := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	if roundTripped != pubKeyStr {
		t.Errorf("public key mismatch after round-trip\ngot:  %s\nwant: %s", roundTripped, pubKeyStr)
	}

	if !strings.HasPrefix(pubKeyStr, "ssh-rsa ") {
		t.Errorf("expected public key to start with \"ssh-rsa \", got: %s", pubKeyStr)
	}
}

func TestGenerateHubSSHKeypairDefaultIsEd25519(t *testing.T) {
	_, pubKeyStr, err := generateHubSSHKeypair("")
	if err != nil {
		t.Fatalf("generateHubSSHKeypair(\"\") returned error: %v", err)
	}

	if !strings.HasPrefix(pubKeyStr, "ssh-ed25519 ") {
		t.Errorf("expected empty keyType to default to ed25519, got public key: %s", pubKeyStr)
	}
}
