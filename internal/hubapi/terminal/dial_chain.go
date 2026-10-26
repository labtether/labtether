package terminal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/terminal"
)

const hopDialTimeout = 6 * time.Second

// HopProgressFn is called after each hop connects successfully.
// hopIndex is zero-based; hopCount is total number of intermediate hops (excluding target).
type HopProgressFn func(hopIndex, hopCount int, hopHost string)

// DialSSHChain connects through an ordered chain of SSH hops before reaching the target.
//
// If hops is empty, it dials the target directly (equivalent to a regular ssh.Dial).
// Returns the final SSH client and a slice of all intermediate clients (for cleanup).
// Callers must close the root intermediate first when tearing down the chain.
func DialSSHChain(
	hops []terminal.ResolvedHop,
	target terminal.ResolvedHop,
	onHopConnected HopProgressFn,
	sleepFn func(time.Duration),
) (*ssh.Client, []*ssh.Client, error) {
	if target.ClientConfig == nil {
		return nil, nil, errors.New("target ssh client config is required")
	}
	if target.Addr == "" {
		return nil, nil, errors.New("target address is required")
	}

	if len(hops) == 0 {
		// Direct dial — no intermediate hops.
		client, err := dialDirectSSH(target.Addr, target.ClientConfig, hopDialTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("direct ssh dial to %s failed: %w", target.Addr, err)
		}
		return client, nil, nil
	}

	intermediates := make([]*ssh.Client, 0, len(hops))

	closeAll := func() { CloseIntermediateClients(intermediates) }

	// Dial the first hop directly.
	first := hops[0]
	if first.ClientConfig == nil {
		return nil, nil, fmt.Errorf("hop 0 (%s) has nil client config", first.Addr)
	}
	firstClient, err := dialDirectSSH(first.Addr, first.ClientConfig, hopDialTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial to hop 0 (%s) failed: %w", first.Addr, err)
	}
	intermediates = append(intermediates, firstClient)
	log.Printf("terminal-ssh-chain: hop 0/%d connected to %s", len(hops), first.Addr) // #nosec G706 -- Hop addresses come from resolved operator config, not raw browser input.
	if onHopConnected != nil {
		onHopConnected(0, len(hops), first.Addr)
	}

	// Dial subsequent hops through the previous hop's client.
	for i := 1; i < len(hops); i++ {
		hop := hops[i]
		if hop.ClientConfig == nil {
			closeAll()
			return nil, nil, fmt.Errorf("hop %d (%s) has nil client config", i, hop.Addr)
		}
		prevClient := intermediates[i-1]

		netConn, err := dialWithTimeout(prevClient, hop.Addr, hopDialTimeout)
		if err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("tunnel dial to hop %d (%s) failed: %w", i, hop.Addr, err)
		}

		handshakeCtx, cancelHandshake := context.WithTimeout(context.Background(), hopDialTimeout)
		client, err := handshakeSSHThroughChain(handshakeCtx, netConn, hop.Addr, hop.ClientConfig, firstClient)
		cancelHandshake()
		if err != nil {
			closeAll()
			_ = netConn.Close()
			return nil, nil, fmt.Errorf("ssh handshake to hop %d (%s) failed: %w", i, hop.Addr, err)
		}

		intermediates = append(intermediates, client)
		log.Printf("terminal-ssh-chain: hop %d/%d connected to %s", i, len(hops), hop.Addr) // #nosec G706 -- Hop addresses come from resolved operator config, not raw browser input.
		if onHopConnected != nil {
			onHopConnected(i, len(hops), hop.Addr)
		}
	}

	// Dial the target through the last hop.
	lastClient := intermediates[len(intermediates)-1]
	targetConn, err := dialWithTimeout(lastClient, target.Addr, hopDialTimeout)
	if err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("tunnel dial to target (%s) through chain failed: %w", target.Addr, err)
	}

	handshakeCtx, cancelHandshake := context.WithTimeout(context.Background(), hopDialTimeout)
	finalClient, err := handshakeSSHThroughChain(handshakeCtx, targetConn, target.Addr, target.ClientConfig, firstClient)
	cancelHandshake()
	if err != nil {
		closeAll()
		_ = targetConn.Close()
		return nil, nil, fmt.Errorf("ssh handshake to target (%s) failed: %w", target.Addr, err)
	}

	return finalClient, intermediates, nil
}

// A tunnel's Close sends a channel-close packet; it cannot force a stalled
// jump host to acknowledge it. Cancel the owned TCP-backed root too, so local
// handshake reads and writes unblock without any response from that host.
func handshakeSSHThroughChain(ctx context.Context, conn net.Conn, address string, config *ssh.ClientConfig, root *ssh.Client) (*ssh.Client, error) {
	stopRootClose := context.AfterFunc(ctx, func() { _ = root.Close() })
	client, err := securityruntime.HandshakeSSHContext(ctx, conn, address, config)
	if !stopRootClose() {
		if client != nil {
			_ = client.Close()
		}
		return nil, ctx.Err()
	}
	return client, err
}

func dialDirectSSH(address string, config *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	host, portRaw, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return securityruntime.DialOutboundSSHContext(ctx, host, port, config, timeout)
}

func dialWithTimeout(client *ssh.Client, addr string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.DialContext(ctx, "tcp", addr)
}

// CloseIntermediateClients tears down an owned chain from its TCP-backed root.
// Closing a tunneled client first can wait forever on a stalled outer transport.
func CloseIntermediateClients(clients []*ssh.Client) {
	for _, client := range clients {
		if client != nil {
			_ = client.Close()
		}
	}
}
