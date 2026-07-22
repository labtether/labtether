package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxConnectedResponseBytes = 1 << 20

type connectedAgentsResponse struct {
	Assets []string `json:"assets"`
}

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("connected-agent endpoint returned HTTP %d", e.code)
}

func parseHubBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse hub base URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("hub base URL must use https")
	}
	if parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("hub base URL must be an origin without credentials, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("hub base URL must not contain a path")
	}
	parsed.Path = ""
	return parsed, nil
}

func newStrictHTTPClient(caFile string) (*http.Client, error) {
	caFile = strings.TrimSpace(caFile)
	if caFile == "" {
		return nil, fmt.Errorf("CA file is required; insecure TLS is not supported")
	}
	caPEM, err := os.ReadFile(caFile) // #nosec G304 -- CI operator supplies the explicit CA path.
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA file does not contain a valid certificate")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("redirects are not allowed for the authenticated agent-presence probe")
		},
	}, nil
}

func connectedAgentPresent(ctx context.Context, client *http.Client, endpoint *url.URL, token, assetID string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, fmt.Errorf("create connected-agent request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxConnectedResponseBytes))
		return false, &statusError{code: response.StatusCode}
	}

	var payload connectedAgentsResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxConnectedResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return false, fmt.Errorf("decode connected-agent response: %w", err)
	}
	for _, connectedAssetID := range payload.Assets {
		if connectedAssetID == assetID {
			return true, nil
		}
	}
	return false, nil
}

func waitForConnectedAgent(
	ctx context.Context,
	client *http.Client,
	endpoint *url.URL,
	token string,
	assetID string,
	pollInterval time.Duration,
) error {
	var lastErr error
	for {
		present, err := connectedAgentPresent(ctx, client, endpoint, token, assetID)
		if present {
			return nil
		}
		if err != nil {
			lastErr = err
			var httpErr *statusError
			if errors.As(err, &httpErr) && (httpErr.code == http.StatusUnauthorized || httpErr.code == http.StatusForbidden) {
				return fmt.Errorf("agent-presence authentication failed: %w", err)
			}
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return fmt.Errorf("agent %q did not connect before timeout (last probe: %v)", assetID, lastErr)
			}
			return fmt.Errorf("agent %q did not connect before timeout", assetID)
		case <-timer.C:
		}
	}
}

func run() error {
	baseURLFlag := flag.String("base-url", "", "HTTPS hub origin")
	caFileFlag := flag.String("ca-file", "", "CA certificate used to verify the hub")
	assetIDFlag := flag.String("asset-id", "", "exact connected asset ID to require")
	timeoutFlag := flag.Duration("timeout", 60*time.Second, "maximum time to wait")
	flag.Parse()

	baseURL, err := parseHubBaseURL(*baseURLFlag)
	if err != nil {
		return err
	}
	assetID := strings.TrimSpace(*assetIDFlag)
	if assetID == "" {
		return fmt.Errorf("asset ID is required")
	}
	token := strings.TrimSpace(os.Getenv("LABTETHER_API_TOKEN"))
	if token == "" {
		return fmt.Errorf("LABTETHER_API_TOKEN is required")
	}
	if *timeoutFlag <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	client, err := newStrictHTTPClient(*caFileFlag)
	if err != nil {
		return err
	}

	endpoint := baseURL.ResolveReference(&url.URL{Path: "/agents/connected"})
	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()
	if err := waitForConnectedAgent(ctx, client, endpoint, token, assetID, time.Second); err != nil {
		return err
	}
	fmt.Printf("connected-agent gate passed: %s\n", assetID)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "connected-agent gate failed: %v\n", err)
		os.Exit(1)
	}
}
