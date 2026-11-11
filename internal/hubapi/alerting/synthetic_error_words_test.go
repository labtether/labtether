package alerting

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

// A down HTTP check read `Get "http://…": dial tcp …: connect: connection
// refused`. Each failure is said in words with the host.
func TestSyntheticErrorWordsSayWhatHappened(t *testing.T) {
	dial := func(errno error) error {
		return &url.Error{Op: "Get", URL: "http://192.0.2.10:18999/", Err: &net.OpError{Op: "dial", Net: "tcp",
			Err: &os.SyscallError{Syscall: "connect", Err: errno}}}
	}
	cases := []struct {
		err  error
		want string
	}{
		{dial(syscall.ECONNREFUSED), "192.0.2.10:18999 refused the connection."},
		{dial(syscall.EHOSTUNREACH), "192.0.2.10:18999 can't be reached from the Hub (no route)."},
		{fmt.Errorf("resolve outbound host %q: %w", "nas01.example", &net.DNSError{Name: "nas01.example", IsNotFound: true}), "nas01.example was not found in DNS."},
		{&url.Error{Op: "Get", URL: "http://192.0.2.10:18999/", Err: context.DeadlineExceeded}, "192.0.2.10:18999 did not answer within 10s."},
		{x509.UnknownAuthorityError{}, "192.0.2.10:18999's certificate is not signed by a trusted authority."},
		{x509.CertificateInvalidError{Reason: x509.Expired}, "192.0.2.10:18999's certificate has expired or is not valid yet."},
		{fmt.Errorf("outbound private host %q is not allowed", "192.0.2.10"), `Could not reach 192.0.2.10:18999: outbound private host "192.0.2.10" is not allowed.`},
	}
	for _, tc := range cases {
		if got := syntheticErrorWords("192.0.2.10:18999", 10*time.Second, tc.err); got != tc.want {
			t.Errorf("%v\n got %q\nwant %q", tc.err, got, tc.want)
		}
	}
	if _, _, err := parseSyntheticHostPort("nas01"); syntheticErrorWords("nas01", 0, err) != "The target must be host:port, such as nas01:443." {
		t.Errorf("bad target = %q", syntheticErrorWords("nas01", 0, err))
	}
}
