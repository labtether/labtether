package terminal

import (
	"io"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSH channels have no write deadline. A peer that stops consuming stdin (or
// answering a control request) must not retain the browser handler forever.
// Closing the owned chain from its TCP-backed root unblocks channel flow
// control and network I/O, even when a jump host stops reading its channels.
func handleSSHInput(session *ssh.Session, stdin io.Writer, payload []byte, closeChain func() error, timeout time.Duration) (handled bool, err error) {
	timedOut := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		close(timedOut)
		_ = closeChain()
	})
	handled, err = HandleTerminalControlMessage(session, stdin, payload)
	if !handled && err == nil {
		_, err = stdin.Write(payload)
	}
	if !timer.Stop() {
		<-timedOut
		return handled, os.ErrDeadlineExceeded
	}
	return handled, err
}
