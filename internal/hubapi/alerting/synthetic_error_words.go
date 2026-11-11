package alerting

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// syntheticErrorWords says why a health check could not reach its target, in
// words and with the host, instead of Go's error text such as
// `Get "http://…": dial tcp 192.0.2.10:8080: connect: connection refused`.
func syntheticErrorWords(host string, timeout time.Duration, err error) string {
	if err == nil {
		return ""
	}
	var targetErr syntheticTargetError
	if errors.As(err, &targetErr) {
		return string(targetErr)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "The target"
	}
	var dnsErr *net.DNSError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var invalidCert x509.CertificateInvalidError
	var notTLS tls.RecordHeaderError
	var netErr net.Error
	switch {
	case errors.As(err, &dnsErr):
		name := strings.TrimSpace(dnsErr.Name)
		if name == "" {
			name = host
		}
		if dnsErr.IsNotFound {
			return fmt.Sprintf("%s was not found in DNS.", name)
		}
		if dnsErr.IsTimeout {
			return fmt.Sprintf("The DNS lookup for %s timed out.", name)
		}
		return fmt.Sprintf("The DNS lookup for %s failed.", name)
	case errors.Is(err, syscall.ECONNREFUSED):
		return fmt.Sprintf("%s refused the connection.", host)
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return fmt.Sprintf("%s can't be reached from the Hub (no route).", host)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		if timeout > 0 {
			return fmt.Sprintf("%s did not answer within %s.", host, timeout)
		}
		return fmt.Sprintf("%s did not answer in time.", host)
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Sprintf("%s closed the connection.", host)
	case errors.As(err, &unknownAuthority):
		return fmt.Sprintf("%s's certificate is not signed by a trusted authority.", host)
	case errors.As(err, &hostnameErr):
		return fmt.Sprintf("%s's certificate is for a different name.", host)
	case errors.As(err, &invalidCert):
		if invalidCert.Reason == x509.Expired {
			return fmt.Sprintf("%s's certificate has expired or is not valid yet.", host)
		}
		return fmt.Sprintf("%s's certificate is not valid.", host)
	case errors.As(err, &notTLS):
		return fmt.Sprintf("%s did not answer with TLS.", host)
	case strings.Contains(err.Error(), "tls:"):
		return fmt.Sprintf("The TLS handshake with %s failed.", host)
	}
	// Anything else (for example the Hub's outbound policy) keeps its own
	// words, without Go's request and dial prefixes.
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		err = opErr.Err
	}
	return fmt.Sprintf("Could not reach %s: %s.", host, strings.TrimSuffix(err.Error(), "."))
}

// syntheticTargetError is a check whose target cannot be used, already in words.
type syntheticTargetError string

func (e syntheticTargetError) Error() string { return string(e) }

// syntheticURLHost is the host and port of an HTTP check's URL.
func syntheticURLHost(target string) string {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed.Host == "" {
		return strings.TrimSpace(target)
	}
	return parsed.Host
}
