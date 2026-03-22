package terminal

import (
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/crypto/ssh"

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
// Callers must close intermediate clients in reverse order when done.
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
		client, err := ssh.Dial("tcp", target.Addr, target.ClientConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("direct ssh dial to %s failed: %w", target.Addr, err)
		}
		return client, nil, nil
	}

	intermediates := make([]*ssh.Client, 0, len(hops))

	// closeAll closes all intermediate clients in reverse order.
	closeAll := func() {
		for i := len(intermediates) - 1; i >= 0; i-- {
			_ = intermediates[i].Close()
		}
	}

	// Dial the first hop directly.
	first := hops[0]
	if first.ClientConfig == nil {
		return nil, nil, fmt.Errorf("hop 0 (%s) has nil client config", first.Addr)
	}
	firstClient, err := ssh.Dial("tcp", first.Addr, first.ClientConfig)
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

		sshConn, chans, reqs, err := ssh.NewClientConn(netConn, hop.Addr, hop.ClientConfig)
		if err != nil {
			_ = netConn.Close()
			closeAll()
			return nil, nil, fmt.Errorf("ssh handshake to hop %d (%s) failed: %w", i, hop.Addr, err)
		}

		client := ssh.NewClient(sshConn, chans, reqs)
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

	sshConn, chans, reqs, err := ssh.NewClientConn(targetConn, target.Addr, target.ClientConfig)
	if err != nil {
		_ = targetConn.Close()
		closeAll()
		return nil, nil, fmt.Errorf("ssh handshake to target (%s) failed: %w", target.Addr, err)
	}

	finalClient := ssh.NewClient(sshConn, chans, reqs)
	return finalClient, intermediates, nil
}

func dialWithTimeout(client *ssh.Client, addr string, timeout time.Duration) (net.Conn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := client.Dial("tcp", addr)
		ch <- dialResult{conn, err}
	}()
	select {
	case res := <-ch:
		return res.conn, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("dial to %s timed out after %s", addr, timeout)
	}
}

// CloseIntermediateClients closes all intermediate SSH clients in reverse order.
func CloseIntermediateClients(clients []*ssh.Client) {
	for i := len(clients) - 1; i >= 0; i-- {
		if clients[i] != nil {
			_ = clients[i].Close()
		}
	}
}
