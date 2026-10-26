package operations

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

func TestLocalCommandContextChild(t *testing.T) {
	if os.Getenv("LABTETHER_TEST_CONTEXT_CHILD") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("LABTETHER_TEST_CONTEXT_READY"), []byte("started"), 0600); err != nil {
		os.Exit(2)
	}
	time.Sleep(5 * time.Second) // Safety bound even when the old owner ignores cancellation.
	os.Exit(0)
}

func TestLocalCommandContextStopsRunningChild(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "started")
	t.Setenv("LABTETHER_TEST_CONTEXT_CHILD", "1")
	t.Setenv("LABTETHER_TEST_CONTEXT_READY", ready)
	t.Setenv("LABTETHER_EXEC_ALLOWED_BINARIES", filepath.Base(executable))
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_PREFIXES", securityruntime.QuoteCommandArgv([]string{executable}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := ExecuteLocalCommandContext(ctx, terminal.CommandJob{
			Command: securityruntime.QuoteCommandArgv([]string{executable, "-test.run=^TestLocalCommandContextChild$"}),
		}, CommandExecutorConfig{Timeout: 10 * time.Second, MaxOutputBytes: 1024})
		result <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("child did not start: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "outcome unknown") {
			t.Fatalf("cancellation result = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running child outlived command cancellation")
	}
}

func TestSSHCommandContextStopsSilentHandshake(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("SSH_STRICT_HOST_KEY", "true")
	for _, name := range []string{"SSH_HOST_KEY", "SSH_PASSWORD", "SSH_PRIVATE_KEY", "SSH_PRIVATE_KEY_B64", "SSH_PRIVATE_KEY_PASSPHRASE"} {
		t.Setenv(name, "")
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	expectedHostKey := string(ssh.MarshalAuthorizedKey(hostKey))
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			accepted := make(chan net.Conn, 1)
			acceptDone := make(chan struct{})
			defer func() {
				_ = listener.Close()
				<-acceptDone
				select {
				case conn := <-accepted:
					_ = conn.Close()
				default:
				}
			}()
			go func() {
				defer close(acceptDone)
				conn, err := listener.Accept()
				if err == nil {
					accepted <- conn
				}
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := 5 * time.Second
			want := context.Canceled
			if mode == "deadline" {
				timeout, want = 100*time.Millisecond, context.DeadlineExceeded
			}
			result := make(chan error, 1)
			go func() {
				_, err := ExecuteSSHCommandContext(ctx, terminal.CommandJob{
					Command: "uptime",
					SSHConfig: &terminal.SSHConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
						User: "fixture", Password: "fixture-only", StrictHostKey: true, HostKey: expectedHostKey},
				}, CommandExecutorConfig{Timeout: timeout, MaxOutputBytes: 1024})
				result <- err
			}()
			select {
			case conn := <-accepted:
				defer conn.Close()
			case err := <-result:
				t.Fatalf("SSH did not reach real peer: %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("SSH did not connect")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, want) {
					t.Fatalf("got %v, want %v", err, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("silent peer retained the canceled SSH command")
			}
		})
	}
}

type cancellationHTTPConnector struct{ endpoint string }

func (c cancellationHTTPConnector) ID() string          { return "cancel-fixture" }
func (c cancellationHTTPConnector) DisplayName() string { return c.ID() }
func (c cancellationHTTPConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{ExecuteActions: true}
}
func (c cancellationHTTPConnector) Discover(context.Context) ([]connectorsdk.Asset, error) {
	return nil, nil
}
func (c cancellationHTTPConnector) TestConnection(context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{}, nil
}
func (c cancellationHTTPConnector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{{ID: "mutate"}}
}
func (c cancellationHTTPConnector) ExecuteAction(ctx context.Context, _ string, _ connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, nil)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		// Production providers can return cancellation as a failed value, nil error.
		return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
	}
	defer response.Body.Close()
	return connectorsdk.ActionResult{Status: "succeeded"}, nil
}

func TestConnectorCommandContextCancelsActualRequest(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
		close(closed)
	}))
	defer server.Close()
	defer close(release)
	registry := connectorsdk.NewRegistry()
	registry.Register(cancellationHTTPConnector{endpoint: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan actions.Result, 1)
	go func() {
		result <- ExecuteActionInProcessContext(ctx, actions.Job{Type: actions.RunTypeConnectorAction,
			ConnectorID: "cancel-fixture", ActionID: "mutate", Target: "fixture"}, registry)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("connector request did not start")
	}
	cancel()
	select {
	case got := <-result:
		if got.Status != actions.StatusFailed || !strings.Contains(got.Error, "outcome unknown") {
			t.Fatalf("accepted request result = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("connector request ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("server connection remained open")
	}
}

func TestUpdateContextStopsBeforeNextTarget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var dispatched []string
	result := ExecuteUpdateWithExecutorContext(ctx, updates.Job{Plan: updates.Plan{
		Targets: []string{"first", "must-not-start"}, Scopes: []string{updates.ScopeOSPackages},
	}}, func(operation context.Context, _ updates.Job, target, scope string) updates.RunResultEntry {
		if operation != ctx {
			t.Fatal("executor lost the claim context")
		}
		dispatched = append(dispatched, target)
		cancel()
		return updates.RunResultEntry{Target: target, Scope: scope, Status: updates.StatusFailed}
	})
	if fmt.Sprint(dispatched) != "[first]" || len(result.Results) != 1 || !strings.Contains(result.Summary, "outcome unknown") {
		t.Fatalf("dispatch=%v result=%+v", dispatched, result)
	}
}
