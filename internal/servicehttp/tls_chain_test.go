package servicehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/labtether/labtether/internal/certmgr"
)

// A Hub reached by IP address gets no server name in the TLS hello. It then
// served only the certificate file's leaf: the built-in CA was missing from
// the chain (the iPhone app reads the chain, so it could not show or pin the
// CA the console shows), and a renewed certificate was not served until a
// restart. Every client must get what GetCertificate returns.
func TestRunServesGetCertificateToClientsWithoutServerName(t *testing.T) {
	port := freeTCPPort(t)
	certs, err := certmgr.Provision(t.TempDir(), "127.0.0.1")
	if err != nil {
		t.Fatalf("provision test certificate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:           "tls-chain-test",
			BindAddress:    "127.0.0.1",
			Port:           fmt.Sprintf("%d", port),
			TLSCertFile:    certs.ServerCertPath,
			TLSKeyFile:     certs.ServerKeyPath,
			GetCertificate: certs.Reloader.GetCertificate,
		})
	}()
	waitForTCPServer(t, port)
	chain := func(serverName string) []*x509.Certificate {
		t.Helper()
		// The test reads the chain as sent; it does not verify it.
		conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{InsecureSkipVerify: true, ServerName: serverName}) // #nosec G402 -- test reads the served chain.
		if err != nil {
			t.Fatalf("dial (server name %q): %v", serverName, err)
		}
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates
	}
	for _, serverName := range []string{"", "hub.example.test"} {
		got := chain(serverName)
		if len(got) != 2 || got[0].IsCA || !got[1].IsCA {
			t.Fatalf("server name %q: got %d certificates, want the Hub certificate then its CA", serverName, len(got))
		}
	}
	before := chain("")[0].SerialNumber
	if err := certs.Reloader.ForceRenew(); err != nil {
		t.Fatalf("renew: %v", err)
	}
	after := chain("")
	if after[0].SerialNumber.Cmp(before) == 0 || len(after) != 2 {
		t.Fatalf("after a renewal a client without a server name still got the old certificate, or lost the CA (%d certificates)", len(after))
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
