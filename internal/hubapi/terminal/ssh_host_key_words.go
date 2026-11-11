package terminal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

// The sentences people read about SSH host keys and SSH logins in the
// terminal and in Quick connect. They say what happened and what to do.

func hostPort(host string, port int) string {
	if port <= 0 {
		port = 22
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func hostKeyUnknownMessage(host string, port int) string {
	return fmt.Sprintf("LabTether does not know the SSH host key of %s yet, so it did not connect. "+
		"Connect to it once from the Terminal page in the console to check the key and trust it.", hostPort(host, port))
}

func hostKeyChangedMessage(host string, port int, saved, shown string) string {
	return fmt.Sprintf("The SSH host key of %s changed. LabTether saved %s; the host now shows %s. "+
		"LabTether did not connect and sent no password or key.", hostPort(host, port), saved, shown)
}

func hostKeyAlreadySavedMessage(host string, port int) string {
	return fmt.Sprintf("LabTether already has another SSH host key for %s. Remove the saved key first.", hostPort(host, port))
}

func hostKeyKnownHostsMessage(host string, port int) string {
	return fmt.Sprintf("The Hub's known_hosts file lists another SSH host key for %s, so LabTether does not trust this one.", hostPort(host, port))
}

// hostKeyReadFailedMessage says why reading a host's key failed.
func hostKeyReadFailedMessage(host string, port int, err error) string {
	why := "it did not answer as an SSH server"
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		why = "it refused the connection"
	case errors.As(err, &dnsErr):
		why = "its name was not found"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		why = "it did not answer in time"
	case errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH):
		why = "it cannot be reached from the Hub"
	}
	return fmt.Sprintf("LabTether could not read the SSH host key of %s: %s.", hostPort(host, port), why)
}

const (
	bookmarkNoLoginMessage = "This bookmark has no saved login. Edit the bookmark and choose one, or connect with Quick connect."
	noTerminalRouteMessage = "This device has no connected agent and no SSH login, so the Hub cannot open a terminal on it. " +
		"Start the agent on the device, or add SSH with a login in the device's Connect panel."
)

// errHostKeyUnknown marks a strict SSH login with no saved host key while the
// Hub has no known_hosts file; its text is hostKeyUnknownMessage.
type errHostKeyUnknown struct{ text string }

func (e errHostKeyUnknown) Error() string { return e.text }
