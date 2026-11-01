package testutil

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// SSHHostSigner returns a new SSH host key: "ecdsa" (P-256) or ed25519.
func SSHHostSigner(t *testing.T, kind string) ssh.Signer {
	t.Helper()
	var private any
	var err error
	if kind == "ecdsa" {
		private, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		_, private, err = ed25519.GenerateKey(rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// AuthorizedHostKey returns signer's public key in authorized_keys form.
func AuthorizedHostKey(signer ssh.Signer) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}

// SSHHandshake connects clientConfig to a local SSH server holding hostKeys
// (no client authentication) and returns the client's handshake error.
func SSHHandshake(t *testing.T, clientConfig *ssh.ClientConfig, hostKeys ...ssh.Signer) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	for _, key := range hostKeys {
		serverConfig.AddHostKey(key)
	}
	go func() {
		accepted, err := listener.Accept()
		if err != nil {
			return
		}
		defer accepted.Close()
		if _, channels, requests, err := ssh.NewServerConn(accepted, serverConfig); err == nil {
			go ssh.DiscardRequests(requests)
			for channel := range channels {
				_ = channel.Reject(ssh.Prohibited, "test server")
			}
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), clientConfig)
	if client != nil {
		_ = client.Close()
	}
	return err
}
