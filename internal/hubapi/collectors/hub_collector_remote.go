package collectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/securityruntime"
)

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

	addr := net.JoinHostPort(host, portStr)
	client, err := ssh.Dial("tcp", addr, sshConfig)
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

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

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
		output := stdout.String()
		if err != nil {
			errOutput := stderr.String()
			if errOutput != "" {
				output = output + "\n" + errOutput
			}
			d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("script failed: %v", err))
		} else {
			d.UpdateCollectorStatus(collector.ID, "ok", "")
		}

		// Store output as log event
		if d.LogStore != nil && output != "" {
			_ = d.LogStore.AppendEvent(logs.Event{
				AssetID: collector.AssetID,
				Source:  "hub-collector",
				Level:   "info",
				Message: output,
			})
		}

		// Try to parse output as structured telemetry
		d.ingestCollectorTelemetry(collector, output)
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

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if readErr != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("failed to read API response body: %v", readErr))
		return
	}
	output := string(body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		d.UpdateCollectorStatus(collector.ID, "ok", "")
	} else {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("API returned status %d", resp.StatusCode))
	}

	// Store output as log event
	if d.LogStore != nil && output != "" {
		_ = d.LogStore.AppendEvent(logs.Event{
			AssetID: collector.AssetID,
			Source:  "hub-collector-api",
			Level:   "info",
			Message: output,
		})
	}

	// Try to parse output as structured telemetry
	d.ingestCollectorTelemetry(collector, output)
}

func BuildCollectorSSHHostKeyCallback(config map[string]any) (ssh.HostKeyCallback, bool, error) {
	strictHostKey := true
	if strict, ok := collectorConfigBool(config, "strict_host_key"); ok {
		strictHostKey = strict
	} else if skip, ok := collectorConfigBool(config, "insecure_skip_host_key_verify"); ok {
		strictHostKey = !skip
	}

	expectedHostKey := CollectorConfigString(config, "host_key")
	if !strictHostKey {
		// #nosec G106 -- explicit operator override for non-production/self-signed environments.
		return ssh.InsecureIgnoreHostKey(), true, nil //nolint:gosec // #nosec G106 -- explicit operator override
	}
	if expectedHostKey == "" {
		knownHostsCallback, err := shared.BuildKnownHostsHostKeyCallback()
		if err != nil {
			return nil, false, fmt.Errorf("strict host key enabled but no host_key provided and no known_hosts file is available")
		}
		return knownHostsCallback, false, nil
	}

	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := strings.TrimSpace(ssh.FingerprintSHA256(key))
		if strings.EqualFold(fingerprint, expectedHostKey) {
			return nil
		}

		encoded := base64.StdEncoding.EncodeToString(key.Marshal())
		if strings.EqualFold(encoded, expectedHostKey) {
			return nil
		}

		return fmt.Errorf("host key mismatch")
	}, false, nil
}
