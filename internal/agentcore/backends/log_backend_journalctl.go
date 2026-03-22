package backends

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

const journalQueryCommandTimeout = 20 * time.Second

// LinuxLogBackend implements LogBackend using journalctl.
type LinuxLogBackend struct{}

var (
	// JournalLookPath is the function used to find journalctl. Overridable for tests.
	JournalLookPath = exec.LookPath
	// NewJournalCommandContext is the function used to build journal commands. Overridable for tests.
	NewJournalCommandContext = securityruntime.NewCommandContext
	// JournalQueryTimeout is the timeout for journal queries. Overridable for tests.
	JournalQueryTimeout = journalQueryCommandTimeout
)

// QueryEntries runs a historical journal query and returns entries.
func (LinuxLogBackend) QueryEntries(req agentmgr.JournalQueryData) ([]agentmgr.LogStreamData, error) {
	if _, err := JournalLookPath("journalctl"); err != nil {
		return nil, fmt.Errorf("journalctl is not available on this host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), JournalQueryTimeout)
	defer cancel()

	args := BuildJournalQueryArgs(req)
	cmd, err := NewJournalCommandContext(ctx, "journalctl", args...)
	if err != nil {
		return nil, fmt.Errorf("journalctl command blocked by runtime policy: %w", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("journalctl query timed out")
		}
		stderr := strings.TrimSpace(string(out))
		if stderr != "" {
			return nil, fmt.Errorf("journalctl query failed: %s", stderr)
		}
		return nil, fmt.Errorf("journalctl query failed: %w", err)
	}

	entries := make([]agentmgr.LogStreamData, 0, NormalizedJournalLimit(req.Limit))
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		entry, ok := ParseJournalLine(scanner.Bytes())
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse journal output: %w", err)
	}

	return entries, nil
}

// StreamEntries streams journal entries via `journalctl -f`.
func (LinuxLogBackend) StreamEntries(ctx context.Context, emit func(agentmgr.LogStreamData)) error {
	if _, err := JournalLookPath("journalctl"); err != nil {
		return ErrLogStreamingUnsupported
	}

	cmd, err := NewJournalCommandContext(ctx, "journalctl", "-f", "--output=json", "-n", "0")
	if err != nil {
		return fmt.Errorf("failed to build journalctl stream command: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to open journalctl stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start journalctl: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			_ = cmd.Wait()
			return nil
		default:
		}

		entry, ok := ParseJournalLine(scanner.Bytes())
		if !ok {
			continue
		}
		emit(entry)
	}

	waitErr := cmd.Wait()
	if scanErr := scanner.Err(); scanErr != nil && ctx.Err() == nil {
		return fmt.Errorf("journalctl scanner error: %w", scanErr)
	}
	if waitErr != nil && ctx.Err() == nil {
		return fmt.Errorf("journalctl stream failed: %w", waitErr)
	}

	return nil
}

// BuildJournalQueryArgs builds the arguments for a journalctl query.
func BuildJournalQueryArgs(req agentmgr.JournalQueryData) []string {
	limit := NormalizedJournalLimit(req.Limit)
	args := []string{
		"--no-pager",
		"--output=json",
		"-n", strconv.Itoa(limit),
		"-r",
	}
	if since := strings.TrimSpace(req.Since); since != "" {
		args = append(args, "--since", since)
	}
	if until := strings.TrimSpace(req.Until); until != "" {
		args = append(args, "--until", until)
	}
	if unit := strings.TrimSpace(req.Unit); unit != "" {
		args = append(args, "-u", unit)
	}
	if priority := strings.TrimSpace(req.Priority); priority != "" {
		args = append(args, "-p", priority)
	}
	if search := strings.TrimSpace(req.Search); search != "" {
		args = append(args, "--grep", search)
	}
	return args
}

// NormalizedJournalLimit clamps the journal query limit to a sane range.
func NormalizedJournalLimit(raw int) int {
	if raw <= 0 {
		return 200
	}
	if raw > 1000 {
		return 1000
	}
	return raw
}
