package collectors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	maxRemoteCollectorOutputBytes     = 8 * 1024 * 1024
	maxPersistedCollectorLogBytes     = 4 * 1024
	collectorLogOutputTruncatedMarker = "\n[collector output truncated]"
)

var errRemoteCollectorOutputLimit = errors.New("remote collector output exceeded safe limit")

// boundedCollectorOutput applies one shared byte ceiling across stdout and
// stderr. SSH may copy both streams concurrently, so the budget and buffers
// are protected by one lock. Returning a short write with an error stops the
// SSH stream instead of continuing to discard attacker-controlled output.
type boundedCollectorOutput struct {
	mu       sync.Mutex
	maxBytes int
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	overflow bool
}

type boundedCollectorStream struct {
	output *boundedCollectorOutput
	stderr bool
}

func newBoundedCollectorOutput(maxBytes int) *boundedCollectorOutput {
	return &boundedCollectorOutput{maxBytes: maxBytes}
}

func (o *boundedCollectorOutput) stdoutWriter() io.Writer {
	return boundedCollectorStream{output: o}
}

func (o *boundedCollectorOutput) stderrWriter() io.Writer {
	return boundedCollectorStream{output: o, stderr: true}
}

func (w boundedCollectorStream) Write(p []byte) (int, error) {
	if w.output == nil {
		return 0, errRemoteCollectorOutputLimit
	}
	o := w.output
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.maxBytes < 0 || o.overflow {
		o.overflow = true
		return 0, errRemoteCollectorOutputLimit
	}
	used := o.stdout.Len() + o.stderr.Len()
	remaining := o.maxBytes - used
	if remaining < 0 {
		remaining = 0
	}
	target := &o.stdout
	if w.stderr {
		target = &o.stderr
	}
	if len(p) > remaining {
		written := 0
		if remaining > 0 {
			written, _ = target.Write(p[:remaining])
		}
		o.overflow = true
		return written, errRemoteCollectorOutputLimit
	}
	return target.Write(p)
}

func (o *boundedCollectorOutput) snapshot(includeStderr bool) (string, bool) {
	if o == nil {
		return "", true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	output := o.stdout.String()
	if includeStderr && o.stderr.Len() > 0 {
		if output != "" && len(output)+o.stderr.Len() < o.maxBytes {
			output += "\n"
		}
		output += o.stderr.String()
	}
	return output, o.overflow
}

func readRemoteCollectorOutput(reader io.Reader, maxBytes int) (string, error) {
	if reader == nil || maxBytes < 0 {
		return "", errRemoteCollectorOutputLimit
	}
	body, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes)+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxBytes {
		return "", errRemoteCollectorOutputLimit
	}
	return string(body), nil
}

func (d *Deps) executeSSHCollector(ctx context.Context, collector hubcollector.Collector) {
	host, _ := collector.Config["host"].(string)
	if host == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing host in config")
		return
	}

	portStr := "22"
	if p, ok := collector.Config["port"].(string); ok && p != "" {
		portStr = p
	} else if p, ok := collector.Config["port"].(float64); ok {
		portStr = fmt.Sprintf("%d", int(p))
	}

	user, _ := collector.Config["user"].(string)
	if user == "" {
		user = "root"
	}
	validatedHost, validatedPort, hostErr := securityruntime.ValidateOutboundHostPort(host, portStr, 22)
	if hostErr != nil {
		d.UpdateCollectorStatus(collector.ID, "error", hostErr.Error())
		return
	}
	host = validatedHost
	portStr = strconv.Itoa(validatedPort)

	script, _ := collector.Config["script"].(string)
	if script == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing script in config")
		return
	}

	// Get SSH key from credential
	var signer ssh.Signer
	credentialID, _ := collector.Config["credential_id"].(string)
	if credentialID != "" && d.SecretsManager != nil && d.CredentialStore != nil {
		cred, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
		if err != nil || !ok {
			d.UpdateCollectorStatus(collector.ID, "error", "credential not found")
			return
		}

		decrypted, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
		if err != nil {
			d.UpdateCollectorStatus(collector.ID, "error", "failed to decrypt credential")
			return
		}

		key, err := ssh.ParsePrivateKey([]byte(decrypted))
		if err != nil {
			d.UpdateCollectorStatus(collector.ID, "error", "failed to parse SSH key")
			return
		}
		signer = key
	}

	// Build SSH config
	hostKeyCallback, insecureHostKey, err := BuildCollectorSSHHostKeyCallback(collector.Config)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", err.Error())
		return
	}
	if insecureHostKey {
		log.Printf("hub collector ssh: WARNING: strict host key verification disabled for %s:%s", host, portStr)
	}
	sshConfig := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	if signer != nil {
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else {
		// Try password from config
		password, _ := collector.Config["password"].(string)
		if password != "" {
			sshConfig.Auth = []ssh.AuthMethod{ssh.Password(password)}
		}
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dialCancel()
	client, err := securityruntime.DialOutboundSSHContext(dialCtx, host, validatedPort, sshConfig, 15*time.Second)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("SSH dial failed: %v", err))
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("SSH session failed: %v", err))
		return
	}
	defer session.Close()

	collectorOutput := newBoundedCollectorOutput(maxRemoteCollectorOutputBytes)
	session.Stdout = collectorOutput.stdoutWriter()
	session.Stderr = collectorOutput.stderrWriter()

	// Use context for timeout
	done := make(chan error, 1)
	go func() {
		done <- session.Run(script)
	}()

	select {
	case <-ctx.Done():
		d.UpdateCollectorStatus(collector.ID, "error", "context cancelled")
		return
	case err := <-done:
		output, overflow := collectorOutput.snapshot(err != nil)
		if overflow {
			d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("collector output exceeded %d byte limit", maxRemoteCollectorOutputBytes))
			return
		}
		if err != nil {
			d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("script failed: %v", err))
			d.appendCollectorOutputLog(collector.AssetID, "hub-collector", "error", output)
			return
		}

		d.appendCollectorOutputLog(collector.AssetID, "hub-collector", "info", output)
		if err := d.ingestCollectorTelemetry(ctx, collector, output); err != nil {
			log.Printf("hub collector: failed to ingest SSH telemetry for collector %s: %v", collector.ID, err)
			d.UpdateCollectorStatus(collector.ID, "error", "telemetry ingest failed")
			return
		}
		d.UpdateCollectorStatus(collector.ID, "ok", "")
	}
}

func (d *Deps) executeAPICollector(ctx context.Context, collector hubcollector.Collector) {
	url, _ := collector.Config["url"].(string)
	if url == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing url in config")
		return
	}

	method, _ := collector.Config["method"].(string)
	if method == "" {
		method = "GET"
	}

	// Build HTTP request
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, method, url, nil)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Apply custom headers
	if headers, ok := collector.Config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	// Apply auth
	authType, _ := collector.Config["auth_type"].(string)
	credentialID, _ := collector.Config["credential_id"].(string)
	secret := ""
	if credentialID != "" && d.SecretsManager != nil && d.CredentialStore != nil {
		cred, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
		if err != nil || !ok {
			d.UpdateCollectorStatus(collector.ID, "error", "credential not found")
			return
		}
		decrypted, err := d.SecretsManager.DecryptString(cred.SecretCiphertext, cred.ID)
		if err != nil {
			d.UpdateCollectorStatus(collector.ID, "error", "failed to decrypt credential")
			return
		}
		secret = decrypted
	}

	switch authType {
	case "bearer":
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
	case "basic":
		apiUser, _ := collector.Config["api_user"].(string)
		if apiUser != "" && secret != "" {
			req.SetBasicAuth(apiUser, secret)
		}
	case "api_key":
		headerName, _ := collector.Config["api_key_header"].(string)
		if headerName == "" {
			headerName = "X-API-Key"
		}
		if secret != "" {
			req.Header.Set(headerName, secret)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("API request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	output, readErr := readRemoteCollectorOutput(resp.Body, maxRemoteCollectorOutputBytes)
	if readErr != nil {
		if errors.Is(readErr, errRemoteCollectorOutputLimit) {
			d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("API response exceeded %d byte limit", maxRemoteCollectorOutputBytes))
		} else {
			d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("failed to read API response body: %v", readErr))
		}
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("API returned status %d", resp.StatusCode))
		d.appendCollectorOutputLog(collector.AssetID, "hub-collector-api", "error", output)
		return
	}

	d.appendCollectorOutputLog(collector.AssetID, "hub-collector-api", "info", output)
	if err := d.ingestCollectorTelemetry(ctx, collector, output); err != nil {
		log.Printf("hub collector: failed to ingest API telemetry for collector %s: %v", collector.ID, err)
		d.UpdateCollectorStatus(collector.ID, "error", "telemetry ingest failed")
		return
	}
	d.UpdateCollectorStatus(collector.ID, "ok", "")
}

func (d *Deps) appendCollectorOutputLog(assetID, source, level, output string) {
	if d.LogStore == nil || output == "" {
		return
	}
	_ = d.LogStore.AppendEvent(logs.Event{
		AssetID: assetID,
		Source:  source,
		Level:   level,
		Message: collectorOutputForLog(output),
	})
}

// collectorOutputForLog keeps diagnostic log records small even when a
// collector legitimately returns a larger telemetry payload. Parsing retains
// the separate maxRemoteCollectorOutputBytes budget; persisting every accepted
// payload wholesale would otherwise let a one-second collector amplify into
// hundreds of GiB of log storage per day. Invalid UTF-8 is normalized because
// PostgreSQL text columns reject it.
func collectorOutputForLog(output string) string {
	output = strings.ToValidUTF8(output, "\uFFFD")
	if len(output) <= maxPersistedCollectorLogBytes {
		return output
	}

	contentBytes := maxPersistedCollectorLogBytes - len(collectorLogOutputTruncatedMarker)
	for contentBytes > 0 && !utf8.ValidString(output[:contentBytes]) {
		contentBytes--
	}
	return output[:contentBytes] + collectorLogOutputTruncatedMarker
}

func BuildCollectorSSHHostKeyCallback(config map[string]any) (ssh.HostKeyCallback, bool, error) {
	strictHostKey := true
	if strict, ok := collectorConfigBool(config, "strict_host_key"); ok {
		strictHostKey = strict
	} else if skip, ok := collectorConfigBool(config, "insecure_skip_host_key_verify"); ok {
		strictHostKey = !skip
	}

	expectedHostKey := CollectorConfigString(config, "host_key")
	callback, err := shared.BuildSSHHostKeyCallback(strictHostKey, expectedHostKey)
	if err != nil {
		return nil, false, fmt.Errorf("strict host key enabled but no host_key provided and no known_hosts file is available")
	}
	return callback, !strictHostKey && shared.InsecureSSHHostKeysAllowed(), nil
}
