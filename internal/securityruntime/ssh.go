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
	if timeout <= 0 {
		timeout = defaultOutboundTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := DialOutboundTCPContext(ctx, host, port, timeout)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	return HandshakeSSHContext(ctx, conn, address, config)
}

// HandshakeSSHContext owns conn until the handshake succeeds. SSH's config
// timeout only covers TCP dialing; NewClientConn can otherwise wait forever
// on a silent peer. For a jump-host channel, its owner must also close the
// TCP-backed root on cancellation: channel.Close only sends a close packet and
// cannot force a stalled jump host to answer it.
func HandshakeSSHContext(ctx context.Context, conn net.Conn, address string, config *ssh.ClientConfig) (*ssh.Client, error) {
	if config == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh client config is required")
	}
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	sshConn, channels, requests, err := ssh.NewClientConn(conn, address, config)
	stopped := stopCancel()
	if !stopped || ctx.Err() != nil {
		_ = conn.Close()
		if sshConn != nil {
			_ = sshConn.Close()
		}
		return nil, ctx.Err()
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(sshConn, channels, requests), nil
}
