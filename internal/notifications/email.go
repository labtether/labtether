package notifications

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	defaultSMTPPort        = 587
	smtpOperationTimeout   = 15 * time.Second
	smtpTLSModeStartTLS    = "starttls"
	smtpTLSModeImplicitTLS = "implicit"
	smtpTLSModeInsecure    = "insecure"
)

var smtpDialOutbound = securityruntime.DialOutboundTCPContext

var smtpTLSConfigForHost = func(serverName string) *tls.Config {
	return &tls.Config{ // #nosec G402 -- certificate verification remains enabled.
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}
}

type EmailAdapter struct{}

func (e *EmailAdapter) Type() string { return ChannelTypeEmail }

func (e *EmailAdapter) Send(parent context.Context, config map[string]any, payload map[string]any) error {
	host, _ := config["smtp_host"].(string)
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("email config missing smtp_host")
	}

	port, err := smtpPortFromConfig(config["smtp_port"])
	if err != nil {
		return err
	}

	user, _ := config["smtp_user"].(string)
	pass, _ := config["smtp_pass"].(string)
	from, _ := config["from"].(string)
	if from == "" {
		from = user
	}

	to, _ := payload["to"].(string)
	if to == "" {
		return fmt.Errorf("email payload missing to")
	}

	subject, _ := payload["subject"].(string)
	body, _ := payload["body"].(string)

	fromAddress, err := mail.ParseAddress(from)
	if err != nil || fromAddress == nil || strings.TrimSpace(fromAddress.Address) == "" {
		return fmt.Errorf("email config invalid from address")
	}
	recipientAddresses, err := mail.ParseAddressList(to)
	if err != nil || len(recipientAddresses) == 0 {
		return fmt.Errorf("email payload invalid recipient address")
	}
	recipients := make([]string, 0, len(recipientAddresses))
	recipientHeaders := make([]string, 0, len(recipientAddresses))
	for _, recipient := range recipientAddresses {
		if recipient == nil || strings.TrimSpace(recipient.Address) == "" {
			return fmt.Errorf("email payload invalid recipient address")
		}
		recipients = append(recipients, recipient.Address)
		recipientHeaders = append(recipientHeaders, recipient.String())
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("email subject contains invalid line breaks")
	}

	message := buildSMTPMessage(fromAddress.String(), strings.Join(recipientHeaders, ", "), subject, body)
	return sendSMTPMessage(parent, host, port, user, pass, fromAddress.Address, recipients, message, config)
}

func sendSMTPMessage(parent context.Context, host string, port int, user, pass, from string, recipients []string, message []byte, config map[string]any) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, smtpOperationTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("email send canceled: %w", err)
	}

	tlsMode, err := smtpTLSMode(config, port)
	if err != nil {
		return err
	}
	insecureAcknowledged := smtpConfigBool(config, "allow_insecure_smtp") && securityruntime.InsecureTransportAllowed()
	if tlsMode == smtpTLSModeInsecure && !insecureAcknowledged {
		return fmt.Errorf("insecure SMTP requires allow_insecure_smtp=true and LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}

	rawConn, err := smtpDialOutbound(ctx, host, port, smtpOperationTimeout)
	if err != nil {
		return smtpContextError(ctx, "email SMTP dial failed", err)
	}
	defer rawConn.Close()
	if err := setSMTPConnectionDeadline(rawConn, ctx); err != nil {
		return fmt.Errorf("email SMTP deadline failed: %w", err)
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = rawConn.SetDeadline(time.Now())
		_ = rawConn.Close()
	})
	defer stopCancellation()

	serverName := strings.Trim(strings.TrimSpace(host), "[]")
	tlsConfig := smtpTLSConfigForHost(serverName)
	if tlsConfig == nil || tlsConfig.InsecureSkipVerify || tlsConfig.ServerName == "" || tlsConfig.MinVersion < tls.VersionTLS12 {
		return fmt.Errorf("email TLS verification configuration is invalid")
	}
	secureConnection := false
	smtpConn := rawConn
	if tlsMode == smtpTLSModeImplicitTLS {
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return smtpServerError(ctx, "email implicit TLS handshake failed", err)
		}
		smtpConn = tlsConn
		secureConnection = true
	}

	client, err := smtp.NewClient(smtpConn, serverName)
	if err != nil {
		return smtpServerError(ctx, "email SMTP greeting failed", err)
	}
	defer client.Close()

	if tlsMode == smtpTLSModeStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			// A channel that selects STARTTLS must never silently downgrade. The
			// insecure escape hatch is valid only with the explicit "insecure"
			// transport mode, so a server capability change cannot weaken a saved
			// secure configuration.
			return fmt.Errorf("email SMTP server does not offer required STARTTLS")
		} else {
			if err := client.StartTLS(tlsConfig); err != nil {
				return smtpServerError(ctx, "email STARTTLS failed", err)
			}
			secureConnection = true
		}
	}

	if (user != "" || pass != "") && !secureConnection {
		return fmt.Errorf("email SMTP credentials require a verified TLS connection")
	}
	if user != "" || pass != "" {
		if user == "" || pass == "" {
			return fmt.Errorf("email SMTP authentication requires both username and password")
		}
		if err := client.Auth(smtp.PlainAuth("", user, pass, serverName)); err != nil {
			return smtpServerError(ctx, "email SMTP authentication failed", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return smtpServerError(ctx, "email SMTP sender rejected", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return smtpServerError(ctx, "email SMTP recipient rejected", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpServerError(ctx, "email SMTP data rejected", err)
	}
	// Send validates/encodes every header-derived field before buildSMTPMessage,
	// and buildSMTPMessage places the normalized body after a fixed CRLF/CRLF
	// separator. Arbitrary body text therefore cannot become an SMTP header.
	if _, err := writer.Write(message); err != nil { // codeql[go/email-injection]
		_ = writer.Close()
		return smtpServerError(ctx, "email SMTP write failed", err)
	}
	if err := writer.Close(); err != nil {
		return smtpServerError(ctx, "email SMTP finalize failed", err)
	}
	if err := client.Quit(); err != nil {
		return smtpServerError(ctx, "email SMTP quit failed", err)
	}

	return nil
}

func buildSMTPMessage(from, to, subject, body string) []byte {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s",
		from,
		to,
		mime.QEncoding.Encode("utf-8", subject),
		time.Now().UTC().Format(time.RFC1123Z),
		body,
	))
}

func smtpTLSMode(config map[string]any, port int) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(payloadString(config, "smtp_tls_mode")))
	if mode == "" {
		if port == 465 {
			return smtpTLSModeImplicitTLS, nil
		}
		return smtpTLSModeStartTLS, nil
	}
	switch mode {
	case smtpTLSModeStartTLS:
		return mode, nil
	case smtpTLSModeImplicitTLS, "implicit_tls", "tls", "smtps":
		return smtpTLSModeImplicitTLS, nil
	case smtpTLSModeInsecure, "none", "plaintext":
		return smtpTLSModeInsecure, nil
	default:
		return "", fmt.Errorf("email config invalid smtp_tls_mode")
	}
}

func smtpConfigBool(config map[string]any, key string) bool {
	switch typed := config[key].(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func setSMTPConnectionDeadline(conn net.Conn, ctx context.Context) error {
	deadline := time.Now().Add(smtpOperationTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	return conn.SetDeadline(deadline)
}

func smtpContextError(ctx context.Context, prefix string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", prefix, ctxErr)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func smtpServerError(ctx context.Context, prefix string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", prefix, ctxErr)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return fmt.Errorf("%s: timeout", prefix)
	}
	// SMTP status text and greetings are controlled by the remote server. Keep
	// the stage for diagnostics but never propagate arbitrary peer text into
	// notification history or logs where reflected credentials could persist.
	return errors.New(prefix)
}

func smtpPortFromConfig(raw any) (int, error) {
	if raw == nil {
		return defaultSMTPPort, nil
	}

	var port int64
	switch typed := raw.(type) {
	case int:
		port = int64(typed)
	case int64:
		port = typed
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) || math.Trunc(typed) != typed || typed < 1 || typed > 65535 {
			return 0, fmt.Errorf("email config invalid smtp_port")
		}
		port = int64(typed)
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return defaultSMTPPort, nil
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("email config invalid smtp_port")
		}
		port = parsed
	default:
		return 0, fmt.Errorf("email config invalid smtp_port")
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("email config smtp_port out of range")
	}
	return int(port), nil
}
