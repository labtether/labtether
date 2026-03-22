package collectors

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/securityruntime"
)

func (d *Deps) executeWinRMCollector(ctx context.Context, collector hubcollector.Collector) {
	host, _ := collector.Config["host"].(string)
	if host == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing host in config")
		return
	}

	portStr := "5985"
	if p, ok := collector.Config["port"].(string); ok && p != "" {
		portStr = p
	} else if p, ok := collector.Config["port"].(float64); ok {
		portStr = fmt.Sprintf("%d", int(p))
	}

	useHTTPS := false
	if v, ok := collector.Config["use_https"].(bool); ok {
		useHTTPS = v
	}
	if portStr == "5986" {
		useHTTPS = true
	}
	validatedHost, validatedPort, hostErr := securityruntime.ValidateOutboundHostPort(host, portStr, 5985)
	if hostErr != nil {
		d.UpdateCollectorStatus(collector.ID, "error", hostErr.Error())
		return
	}
	host = validatedHost
	portStr = strconv.Itoa(validatedPort)
	skipVerify, hasSkipVerify := collectorConfigBool(collector.Config, "skip_verify")
	if !hasSkipVerify {
		skipVerify = false
	}
	caPEM := CollectorConfigString(collector.Config, "ca_pem")

	user, _ := collector.Config["user"].(string)
	if user == "" {
		user = "Administrator"
	}

	script, _ := collector.Config["script"].(string)
	if script == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing script in config")
		return
	}

	// Get password from credential
	password := ""
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
		password = decrypted
	} else if pw, ok := collector.Config["password"].(string); ok {
		password = pw
	}

	if password == "" {
		d.UpdateCollectorStatus(collector.ID, "error", "missing password for WinRM (credential_id or password required)")
		return
	}

	// Build WinRM endpoint URL
	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s:%s/wsman", scheme, host, portStr)

	// Execute via HTTP since we can't add the winrm dependency yet
	// Use a basic WinRM SOAP envelope for command execution
	output, err := ExecuteWinRMCommand(ctx, endpoint, user, password, script, useHTTPS, skipVerify, caPEM)
	if err != nil {
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("WinRM execution failed: %v", err))
	} else {
		d.UpdateCollectorStatus(collector.ID, "ok", "")
	}

	// Store output as log event
	if d.LogStore != nil && output != "" {
		_ = d.LogStore.AppendEvent(logs.Event{
			AssetID: collector.AssetID,
			Source:  "hub-collector-winrm",
			Level:   "info",
			Message: output,
		})
	}

	// Try to parse output as structured telemetry
	d.ingestCollectorTelemetry(collector, output)
}

// ExecuteWinRMCommand performs WinRM command execution via the WS-Management SOAP
// protocol. The full flow is: Create Shell → Execute Command → Receive Output → Delete Shell.
func ExecuteWinRMCommand(ctx context.Context, endpoint, user, password, command string, useHTTPS, skipVerify bool, caPEM string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	if useHTTPS {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		if strings.TrimSpace(caPEM) != "" {
			pool := x509.NewCertPool()
			if ok := pool.AppendCertsFromPEM([]byte(caPEM)); !ok {
				return "", fmt.Errorf("invalid ca_pem certificate bundle")
			}
			tlsConfig.RootCAs = pool
		}
		if skipVerify {
			log.Printf("hub collector winrm: WARNING: TLS certificate verification disabled for endpoint=%s", endpoint)
			tlsConfig.InsecureSkipVerify = true //nolint:gosec // explicit operator override
		}
		client.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	// Step 1: Create Shell
	shellID, err := winrmCreateShell(ctx, client, endpoint, user, password)
	if err != nil {
		return "", fmt.Errorf("create shell: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := winrmDeleteShell(cleanupCtx, client, endpoint, user, password, shellID); err != nil {
			log.Printf("winrm: failed to delete shell %s: %v", shellID, err)
		}
	}()

	// Step 2: Execute Command (PowerShell with EncodedCommand for safe quoting)
	commandID, err := winrmExecuteCommand(ctx, client, endpoint, user, password, shellID, command)
	if err != nil {
		return "", fmt.Errorf("execute command: %w", err)
	}

	// Step 3: Receive Output (poll until command completes)
	stdout, stderr, err := winrmReceiveOutput(ctx, client, endpoint, user, password, shellID, commandID)
	if err != nil {
		return "", fmt.Errorf("receive output: %w", err)
	}

	output := strings.TrimSpace(stdout)
	if stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += strings.TrimSpace(stderr)
	}
	return output, nil
}

const winrmShellURI = "http://schemas.microsoft.com/wbem/wsman/1/windows/shell"
const MaxWinRMSOAPResponseBytes = 8 * 1024 * 1024

// WinRMSOAPRequest sends a SOAP envelope to the WinRM endpoint and returns the response body.
func WinRMSOAPRequest(ctx context.Context, client *http.Client, endpoint, user, password, body string) (string, error) {
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, "POST", endpoint, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(user, password)
	req.Header.Set("Content-Type", "application/soap+xml;charset=UTF-8")

	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxWinRMSOAPResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if len(respBody) > MaxWinRMSOAPResponseBytes {
		return "", fmt.Errorf("response exceeded %d bytes", MaxWinRMSOAPResponseBytes)
	}
	respStr := string(respBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := respStr
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}
	// Check for SOAP faults returned with 200 OK status
	if strings.Contains(respStr, ":Fault>") || strings.Contains(respStr, "<s:Fault>") {
		snippet := respStr
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return "", fmt.Errorf("SOAP fault: %s", snippet)
	}
	return respStr, nil
}

// winrmCreateShell creates a WinRM command shell and returns the ShellId.
func winrmCreateShell(ctx context.Context, client *http.Client, endpoint, user, password string) (string, error) {
	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd">
  <s:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2004/09/transfer/Create</wsa:Action>
    <wsa:To>%s</wsa:To>
    <wsman:ResourceURI>%s</wsman:ResourceURI>
    <wsman:OperationTimeout>PT60S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <rsp:Shell xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell">
      <rsp:InputStreams>stdin</rsp:InputStreams>
      <rsp:OutputStreams>stdout stderr</rsp:OutputStreams>
    </rsp:Shell>
  </s:Body>
</s:Envelope>`, endpoint, winrmShellURI)

	body, err := WinRMSOAPRequest(ctx, client, endpoint, user, password, envelope)
	if err != nil {
		return "", err
	}

	shellID := extractXMLTagValue(body, "ShellId")
	if shellID == "" {
		return "", fmt.Errorf("no ShellId in response")
	}
	return shellID, nil
}

// winrmExecuteCommand sends a command to an existing shell and returns the CommandId.
func winrmExecuteCommand(ctx context.Context, client *http.Client, endpoint, user, password, shellID, command string) (string, error) {
	encoded := encodePowerShellCommand(command)

	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd">
  <s:Header>
    <wsa:Action>http://schemas.microsoft.com/wbem/wsman/1/windows/shell/Command</wsa:Action>
    <wsa:To>%s</wsa:To>
    <wsman:ResourceURI>%s</wsman:ResourceURI>
    <wsman:SelectorSet>
      <wsman:Selector Name="ShellId">%s</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT60S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <rsp:CommandLine xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell">
      <rsp:Command>powershell.exe</rsp:Command>
      <rsp:Arguments>-NoProfile -NonInteractive -EncodedCommand %s</rsp:Arguments>
    </rsp:CommandLine>
  </s:Body>
</s:Envelope>`, endpoint, winrmShellURI, shellID, encoded)

	body, err := WinRMSOAPRequest(ctx, client, endpoint, user, password, envelope)
	if err != nil {
		return "", err
	}

	commandID := extractXMLTagValue(body, "CommandId")
	if commandID == "" {
		return "", fmt.Errorf("no CommandId in response")
	}
	return commandID, nil
}

// winrmReceiveOutput polls for command output until the command completes.
// Returns stdout and stderr as separate strings.
const winrmMaxOutputBytes = 10 * 1024 * 1024 // 10 MB output cap

func winrmReceiveOutput(ctx context.Context, client *http.Client, endpoint, user, password, shellID, commandID string) (string, string, error) {
	var stdoutBuf, stderrBuf strings.Builder

	for i := 0; i < 120; i++ { // max 120 polls (~60 seconds)
		select {
		case <-ctx.Done():
			return stdoutBuf.String(), stderrBuf.String(), ctx.Err()
		default:
		}

		envelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd">
  <s:Header>
    <wsa:Action>http://schemas.microsoft.com/wbem/wsman/1/windows/shell/Receive</wsa:Action>
    <wsa:To>%s</wsa:To>
    <wsman:ResourceURI>%s</wsman:ResourceURI>
    <wsman:SelectorSet>
      <wsman:Selector Name="ShellId">%s</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT10S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <rsp:Receive xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell">
      <rsp:DesiredStream CommandId="%s">stdout stderr</rsp:DesiredStream>
    </rsp:Receive>
  </s:Body>
</s:Envelope>`, endpoint, winrmShellURI, shellID, commandID)

		body, err := WinRMSOAPRequest(ctx, client, endpoint, user, password, envelope)
		if err != nil {
			return stdoutBuf.String(), stderrBuf.String(), err
		}

		// Extract base64-encoded stream chunks from response
		for _, chunk := range extractStreamChunks(body, "stdout") {
			decoded, err := base64.StdEncoding.DecodeString(chunk)
			if err == nil {
				stdoutBuf.Write(decoded)
			}
		}
		for _, chunk := range extractStreamChunks(body, "stderr") {
			decoded, err := base64.StdEncoding.DecodeString(chunk)
			if err == nil {
				stderrBuf.Write(decoded)
			}
		}

		// Cap output size to prevent unbounded memory growth
		if stdoutBuf.Len()+stderrBuf.Len() > winrmMaxOutputBytes {
			return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("output exceeded %d byte limit", winrmMaxOutputBytes)
		}

		// Check if command has completed (WinRM CommandState element with Done URI)
		if strings.Contains(body, "CommandState") && strings.Contains(body, "/Done") {
			return stdoutBuf.String(), stderrBuf.String(), nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command did not complete within 60s timeout")
}

// winrmDeleteShell sends a Delete request to clean up the remote shell.
func winrmDeleteShell(ctx context.Context, client *http.Client, endpoint, user, password, shellID string) error {
	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd">
  <s:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2004/09/transfer/Delete</wsa:Action>
    <wsa:To>%s</wsa:To>
    <wsman:ResourceURI>%s</wsman:ResourceURI>
    <wsman:SelectorSet>
      <wsman:Selector Name="ShellId">%s</wsman:Selector>
    </wsman:SelectorSet>
  </s:Header>
  <s:Body/>
</s:Envelope>`, endpoint, winrmShellURI, shellID)

	_, err := WinRMSOAPRequest(ctx, client, endpoint, user, password, envelope)
	return err
}

// encodePowerShellCommand encodes a command string for PowerShell -EncodedCommand.
// This avoids quoting issues by encoding the script as UTF-16LE base64.
func encodePowerShellCommand(cmd string) string {
	runes := utf16.Encode([]rune(cmd))
	buf := make([]byte, len(runes)*2)
	for i, r := range runes {
		binary.LittleEndian.PutUint16(buf[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// winrmStreamRe matches WinRM SOAP stream elements: <rsp:Stream Name="stdout" ...>base64data</rsp:Stream>
// Handles any namespace prefix (rsp:, w:, etc.) and captures the stream name and base64 data.
var winrmStreamRe = regexp.MustCompile(`<\w*:?Stream[^>]*\bName="([^"]*)"[^>]*>([A-Za-z0-9+/=\s]+)</\w*:?Stream>`)

// extractStreamChunks extracts base64-encoded data from WinRM SOAP Stream elements
// matching the given stream name (e.g., "stdout" or "stderr").
func extractStreamChunks(body, streamName string) []string {
	var chunks []string
	for _, match := range winrmStreamRe.FindAllStringSubmatch(body, -1) {
		if len(match) >= 3 && match[1] == streamName {
			data := strings.TrimSpace(match[2])
			if data != "" {
				chunks = append(chunks, data)
			}
		}
	}
	return chunks
}

// extractXMLTagCache caches compiled regexps keyed by XML local name pattern.
var extractXMLTagCache sync.Map

// extractXMLTagValue extracts the text content of an XML element by local name,
// handling any namespace prefix (e.g., <rsp:ShellId>value</rsp:ShellId>).
func extractXMLTagValue(body, localName string) string {
	pattern := `<[^>]*?` + regexp.QuoteMeta(localName) + `[^>]*?>([^<]+)</`
	var re *regexp.Regexp
	if cached, ok := extractXMLTagCache.Load(pattern); ok {
		re = cached.(*regexp.Regexp)
	} else {
		re = regexp.MustCompile(pattern)
		extractXMLTagCache.Store(pattern, re)
	}
	match := re.FindStringSubmatch(body)
	if len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}
