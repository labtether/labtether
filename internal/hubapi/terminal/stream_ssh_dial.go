package terminal

import (
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"strings"
	"time"
)

const (
	SSHDialAttemptTimeout  = 6 * time.Second
	SSHDialMaxAttempts     = 2
	SSHDialRetryDelay      = 350 * time.Millisecond
	SSHShellStartupTimeout = 8 * time.Second
)

func DialSSHWithRetry(
	addr string,
	baseConfig *ssh.ClientConfig,
	attemptTimeout time.Duration,
	maxAttempts int,
	retryDelay time.Duration,
	dialFn func(network, target string, cfg *ssh.ClientConfig) (*ssh.Client, error),
	sleepFn func(time.Duration),
	onAttempt func(attempt, attempts int),
) (*ssh.Client, int, error) {
	if baseConfig == nil {
		return nil, 0, errors.New("ssh client config is required")
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if dialFn == nil {
		return nil, 0, errors.New("ssh dial function is required")
	}
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if onAttempt != nil {
			onAttempt(attempt, maxAttempts)
		}
		cfg := *baseConfig
		cfg.Timeout = attemptTimeout
		dialStart := time.Now()
		client, err := dialFn("tcp", addr, &cfg)
		if err == nil && client != nil {
			log.Printf("terminal-ssh: connected to %s on attempt %d/%d in %s", addr, attempt, maxAttempts, time.Since(dialStart).Round(time.Millisecond)) // #nosec G706 -- Address comes from resolved operator config; remaining fields are bounded runtime values.
			return client, attempt, nil
		}
		if err == nil {
			err = errors.New("ssh dial returned nil client")
		}
		lastErr = err
		log.Printf("terminal-ssh: attempt %d/%d to %s failed after %s: %v", attempt, maxAttempts, addr, time.Since(dialStart).Round(time.Millisecond), err) // #nosec G706 -- Address comes from resolved operator config; remaining fields are bounded runtime values.
		if attempt < maxAttempts && retryDelay > 0 {
			sleepFn(retryDelay)
		}
	}
	return nil, maxAttempts, fmt.Errorf("ssh dial failed: %w", lastErr)
}

func StartSSHInteractiveSessionWithTimeout(
	sshClient *ssh.Client,
	cols int,
	rows int,
	startCommand string,
	timeout time.Duration,
) (*ssh.Session, io.WriteCloser, io.Reader, io.Reader, error) {
	if sshClient == nil {
		return nil, nil, nil, nil, errors.New("ssh client is required")
	}

	type sessionInitResult struct {
		session *ssh.Session
		stdin   io.WriteCloser
		stdout  io.Reader
		stderr  io.Reader
		err     error
	}

	resultCh := make(chan sessionInitResult, 1)
	go func() {
		sshSession, err := sshClient.NewSession()
		if err != nil {
			resultCh <- sessionInitResult{err: fmt.Errorf("ssh session failed: %w", err)}
			return
		}

		stdin, err := sshSession.StdinPipe()
		if err != nil {
			_ = sshSession.Close()
			resultCh <- sessionInitResult{err: fmt.Errorf("ssh stdin failed: %w", err)}
			return
		}
		stdout, err := sshSession.StdoutPipe()
		if err != nil {
			_ = sshSession.Close()
			resultCh <- sessionInitResult{err: fmt.Errorf("ssh stdout failed: %w", err)}
			return
		}
		stderr, err := sshSession.StderrPipe()
		if err != nil {
			_ = sshSession.Close()
			resultCh <- sessionInitResult{err: fmt.Errorf("ssh stderr failed: %w", err)}
			return
		}

		if err := sshSession.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}); err != nil {
			_ = sshSession.Close()
			resultCh <- sessionInitResult{err: fmt.Errorf("ssh pty request failed: %w", err)}
			return
		}
		if strings.TrimSpace(startCommand) != "" {
			if err := sshSession.Start(startCommand); err != nil {
				_ = sshSession.Close()
				resultCh <- sessionInitResult{err: fmt.Errorf("ssh command start failed: %w", err)}
				return
			}
		} else {
			if err := sshSession.Shell(); err != nil {
				_ = sshSession.Close()
				resultCh <- sessionInitResult{err: fmt.Errorf("ssh shell start failed: %w", err)}
				return
			}
		}

		resultCh <- sessionInitResult{
			session: sshSession,
			stdin:   stdin,
			stdout:  stdout,
			stderr:  stderr,
		}
	}()

	if timeout <= 0 {
		result := <-resultCh
		return result.session, result.stdin, result.stdout, result.stderr, result.err
	}

	select {
	case result := <-resultCh:
		return result.session, result.stdin, result.stdout, result.stderr, result.err
	case <-time.After(timeout):
		if sshClient != nil {
			_ = sshClient.Close()
		}
		go func() {
			if r := <-resultCh; r.session != nil {
				_ = r.session.Close()
			}
		}()
		return nil, nil, nil, nil, fmt.Errorf("ssh shell startup timed out after %s", timeout.Round(time.Second))
	}
}
