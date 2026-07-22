package securityruntime

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

// DialOutboundSSHContext performs policy-bound, single-resolution TCP dialing
// before the SSH handshake. The original hostname remains the SSH callback
// address so known_hosts and certificate checks retain their expected name.
func DialOutboundSSHContext(ctx context.Context, host string, port int, config *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	if config == nil {
		return nil, fmt.Errorf("ssh client config is required")
	}
	conn, err := DialOutboundTCPContext(ctx, host, port, timeout)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	sshConn, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(sshConn, channels, requests), nil
}
