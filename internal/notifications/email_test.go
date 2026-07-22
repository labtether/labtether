package notifications

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestEmailAdapterDeliversThroughVerifiedImplicitTLSSink(t *testing.T) {
	certificate, roots := newSMTPTestCertificate(t, "smtp.example.com")
	originalDial := smtpDialOutbound
	originalTLSConfig := smtpTLSConfigForHost
	t.Cleanup(func() {
		smtpDialOutbound = originalDial
		smtpTLSConfigForHost = originalTLSConfig
	})
	smtpTLSConfigForHost = func(serverName string) *tls.Config {
		return &tls.Config{ // #nosec G402 -- test root verifies the generated local certificate.
			MinVersion: tls.VersionTLS12,
			ServerName: serverName,
			RootCAs:    roots,
		}
	}

	serverResult := make(chan struct {
		message string
		err     error
	}, 1)
	smtpDialOutbound = func(context.Context, string, int, time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			message, err := runImplicitTLSSMTPSink(server, certificate)
			serverResult <- struct {
				message string
				err     error
			}{message: message, err: err}
		}()
		return client, nil
	}

	err := (&EmailAdapter{}).Send(context.Background(), map[string]any{
		"smtp_host":     "smtp.example.com",
		"smtp_port":     465,
		"smtp_tls_mode": "implicit",
		"from":          "LabTether QA <sender@example.com>",
	}, map[string]any{
		"to":      "recipient@example.com",
		"subject": "[CRITICAL] Local TLS proof",
		"body":    "LabTether SMTP/TLS delivery body",
	})
	if err != nil {
		t.Fatalf("deliver through verified implicit TLS sink: %v", err)
	}

	select {
	case result := <-serverResult:
		if result.err != nil {
			t.Fatalf("local SMTP/TLS sink: %v", result.err)
		}
		for _, expected := range []string{
			"To: <recipient@example.com>",
			"Subject: [CRITICAL] Local TLS proof",
			"LabTether SMTP/TLS delivery body",
		} {
			if !strings.Contains(result.message, expected) {
				t.Fatalf("SMTP message missing %q:\n%s", expected, result.message)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("local SMTP/TLS sink did not finish")
	}
}

func newSMTPTestCertificate(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate SMTP test key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create SMTP test certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal SMTP test key: %v", err)
	}
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatalf("parse SMTP test keypair: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse SMTP test certificate: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return certificate, roots
}

func runImplicitTLSSMTPSink(raw net.Conn, certificate tls.Certificate) (string, error) {
	defer raw.Close()
	secure := tls.Server(raw, &tls.Config{ // #nosec G402 -- generated certificate is scoped to this in-memory test sink.
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
	})
	if err := secure.Handshake(); err != nil {
		return "", err
	}
	reader := bufio.NewReader(secure)
	if _, err := io.WriteString(secure, "220 smtp.example.com ESMTP local-test\r\n"); err != nil {
		return "", err
	}

	var message strings.Builder
	inData := false
	quitAccepted := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if quitAccepted && errors.Is(err, io.EOF) {
				return message.String(), nil
			}
			return message.String(), err
		}
		trimmed := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if inData {
			if trimmed == "." {
				inData = false
				if _, err := io.WriteString(secure, "250 2.0.0 queued\r\n"); err != nil {
					return message.String(), err
				}
				continue
			}
			message.WriteString(trimmed)
			message.WriteString("\r\n")
			continue
		}

		command := strings.ToUpper(strings.TrimSpace(trimmed))
		switch {
		case strings.HasPrefix(command, "EHLO "):
			_, err = io.WriteString(secure, "250-smtp.example.com\r\n250 SIZE 65536\r\n")
		case strings.HasPrefix(command, "MAIL FROM:"):
			_, err = io.WriteString(secure, "250 2.1.0 sender ok\r\n")
		case strings.HasPrefix(command, "RCPT TO:"):
			_, err = io.WriteString(secure, "250 2.1.5 recipient ok\r\n")
		case command == "DATA":
			inData = true
			_, err = io.WriteString(secure, "354 end data with <CR><LF>.<CR><LF>\r\n")
		case command == "QUIT":
			_, err = io.WriteString(secure, "221 2.0.0 bye\r\n")
			quitAccepted = err == nil
		default:
			_, err = io.WriteString(secure, "500 5.5.1 unsupported command\r\n")
		}
		if err != nil {
			return message.String(), err
		}
	}
}

func TestSMTPPortFromConfigDefaultsAndAcceptsValidPorts(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int
	}{
		{name: "missing", raw: nil, want: defaultSMTPPort},
		{name: "int", raw: 2525, want: 2525},
		{name: "int64", raw: int64(465), want: 465},
		{name: "float64 integer", raw: float64(587), want: 587},
		{name: "string", raw: " 1025 ", want: 1025},
		{name: "blank string", raw: " ", want: defaultSMTPPort},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := smtpPortFromConfig(tc.raw)
			if err != nil {
				t.Fatalf("smtpPortFromConfig(%v) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("smtpPortFromConfig(%v) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSMTPPortFromConfigRejectsUnsafeValues(t *testing.T) {
	cases := []struct {
		name string
		raw  any
	}{
		{name: "zero", raw: 0},
		{name: "negative", raw: -1},
		{name: "too large int", raw: 65536},
		{name: "fractional", raw: 25.5},
		{name: "huge float", raw: 1e100},
		{name: "infinite", raw: math.Inf(1)},
		{name: "nan", raw: math.NaN()},
		{name: "malformed string", raw: "587abc"},
		{name: "unsupported", raw: []string{"587"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := smtpPortFromConfig(tc.raw); err == nil {
				t.Fatalf("smtpPortFromConfig(%v) = %d, nil error; want rejection", tc.raw, got)
			}
		})
	}
}

func TestEmailAdapterDeniesLoopbackSMTPByDefault(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LINK_LOCAL", "false")

	adapter := &EmailAdapter{}
	err := adapter.Send(context.Background(), map[string]any{
		"smtp_host": "127.0.0.1",
		"smtp_port": 25,
		"from":      "sender@example.com",
	}, map[string]any{
		"to":      "recipient@example.com",
		"subject": "test",
		"body":    "body",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "loopback") {
		t.Fatalf("loopback SMTP error = %v, want outbound-policy denial", err)
	}
}

func TestEmailAdapterCancellationInterruptsStalledSMTPGreeting(t *testing.T) {
	originalDial := smtpDialOutbound
	defer func() { smtpDialOutbound = originalDial }()

	serverConn := make(chan net.Conn, 1)
	smtpDialOutbound = func(context.Context, string, int, time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		serverConn <- server
		return client, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- (&EmailAdapter{}).Send(ctx, map[string]any{
			"smtp_host": "smtp.example.com",
			"smtp_port": 587,
			"from":      "sender@example.com",
		}, map[string]any{
			"to":      "recipient@example.com",
			"subject": "test",
			"body":    "body",
		})
	}()

	stalledServer := <-serverConn
	defer stalledServer.Close()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stalled SMTP cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SMTP send did not stop promptly after context cancellation")
	}
}

func TestEmailAdapterSTARTTLSModeNeverDowngrades(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	originalDial := smtpDialOutbound
	defer func() { smtpDialOutbound = originalDial }()

	serverDone := make(chan struct{})
	smtpDialOutbound = func(context.Context, string, int, time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer close(serverDone)
			defer server.Close()
			_, _ = io.WriteString(server, "220 smtp.example.com ESMTP ready\r\n")
			reader := bufio.NewReader(server)
			if _, err := reader.ReadString('\n'); err != nil {
				return
			}
			_, _ = io.WriteString(server, "250-smtp.example.com\r\n250 SIZE 1024\r\n")
		}()
		return client, nil
	}

	err := (&EmailAdapter{}).Send(context.Background(), map[string]any{
		"smtp_host":           "smtp.example.com",
		"smtp_port":           587,
		"smtp_tls_mode":       "starttls",
		"allow_insecure_smtp": true,
		"from":                "sender@example.com",
	}, map[string]any{
		"to":      "recipient@example.com",
		"subject": "test",
		"body":    "body",
	})
	if err == nil || !strings.Contains(err.Error(), "does not offer required STARTTLS") {
		t.Fatalf("SMTP without STARTTLS error = %v, want mandatory TLS rejection", err)
	}
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("fake SMTP server did not exit")
	}
}

func TestEmailAdapterDoesNotPropagateRemoteSMTPText(t *testing.T) {
	originalDial := smtpDialOutbound
	defer func() { smtpDialOutbound = originalDial }()

	const reflected = "synthetic-reflected-credential"
	smtpDialOutbound = func(context.Context, string, int, time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_, _ = io.WriteString(server, "554 "+reflected+"\r\n")
		}()
		return client, nil
	}

	err := (&EmailAdapter{}).Send(context.Background(), map[string]any{
		"smtp_host": "smtp.example.com",
		"smtp_port": 587,
		"from":      "sender@example.com",
	}, map[string]any{
		"to":      "recipient@example.com",
		"subject": "test",
		"body":    "body",
	})
	if err == nil {
		t.Fatal("expected SMTP greeting rejection")
	}
	if strings.Contains(err.Error(), reflected) {
		t.Fatal("email adapter propagated remote-controlled SMTP response text")
	}
}

func TestEmailAdapterInsecureSMTPRequiresDualAcknowledgement(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")

	adapter := &EmailAdapter{}
	err := adapter.Send(context.Background(), map[string]any{
		"smtp_host":           "smtp.example.com",
		"smtp_port":           25,
		"smtp_tls_mode":       "insecure",
		"allow_insecure_smtp": true,
		"from":                "sender@example.com",
	}, map[string]any{
		"to":      "recipient@example.com",
		"subject": "test",
		"body":    "body",
	})
	if err == nil || !strings.Contains(err.Error(), "requires allow_insecure_smtp=true") {
		t.Fatalf("insecure SMTP acknowledgement error = %v, want dual-acknowledgement rejection", err)
	}
}

func TestEmailAdapterRejectsSubjectHeaderInjectionBeforeDial(t *testing.T) {
	originalDial := smtpDialOutbound
	defer func() { smtpDialOutbound = originalDial }()
	dialed := false
	smtpDialOutbound = func(context.Context, string, int, time.Duration) (net.Conn, error) {
		dialed = true
		return nil, errors.New("unexpected dial")
	}

	err := (&EmailAdapter{}).Send(context.Background(), map[string]any{
		"smtp_host": "smtp.example.com",
		"from":      "sender@example.com",
	}, map[string]any{
		"to":      "recipient@example.com",
		"subject": "valid\r\nBcc: attacker@example.com",
		"body":    "body",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid line breaks") {
		t.Fatalf("subject injection error = %v, want rejection", err)
	}
	if dialed {
		t.Fatal("SMTP dial occurred before subject injection was rejected")
	}
}
