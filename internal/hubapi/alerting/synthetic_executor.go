package alerting

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/synthetic"
)

// SharedSyntheticHTTPTransport is a shared HTTP transport for synthetic checks.
// It enables connection pooling across the 15s check cycle instead of creating
// a new client per check.
var SharedSyntheticHTTPTransport = &http.Transport{
	MaxIdleConns:        20,
	MaxIdleConnsPerHost: 5,
	IdleConnTimeout:     90 * time.Second,
}

// syntheticRegexCache caches compiled regexps for body-match patterns so
// that recurring checks don't re-compile the same pattern every cycle.
var syntheticRegexCache sync.Map // pattern string → *regexp.Regexp

// CachedRegexp returns a compiled regexp for the given pattern, caching the
// result so that recurring checks don't re-compile the same pattern.
func CachedRegexp(pattern string) (*regexp.Regexp, error) {
	if cached, ok := syntheticRegexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	syntheticRegexCache.Store(pattern, re)
	return re, nil
}

// ExecuteSyntheticCheck runs a single check against the target using the
// appropriate protocol (HTTP, TCP, DNS, or TLS certificate).
func ExecuteSyntheticCheck(check synthetic.Check) synthetic.Result {
	switch check.CheckType {
	case synthetic.CheckTypeHTTP:
		return executeHTTPCheck(check)
	case synthetic.CheckTypeTCP:
		return executeTCPCheck(check)
	case synthetic.CheckTypeDNS:
		return executeDNSCheck(check)
	case synthetic.CheckTypeTLSCert:
		return executeTLSCheck(check)
	default:
		now := time.Now().UTC()
		return synthetic.Result{
			CheckID:   check.ID,
			Status:    synthetic.ResultStatusFail,
			Error:     fmt.Sprintf("unsupported check type: %s", check.CheckType),
			CheckedAt: now,
		}
	}
}

func executeHTTPCheck(check synthetic.Check) synthetic.Result {
	start := time.Now()
	timeout := 10 * time.Second
	if t, ok := check.Config["timeout_seconds"]; ok {
		if ts, ok := t.(float64); ok && ts > 0 {
			timeout = time.Duration(ts) * time.Second
		}
	}

	client := &http.Client{Timeout: timeout, Transport: SharedSyntheticHTTPTransport}
	resp, err := client.Get(check.Target)
	latencyMS := int(time.Since(start).Milliseconds())

	if err != nil {
		return synthetic.Result{
			CheckID:   check.ID,
			Status:    synthetic.ResultStatusFail,
			LatencyMS: &latencyMS,
			Error:     err.Error(),
			Metadata:  map[string]any{"check_type": "http", "target": check.Target},
			CheckedAt: time.Now().UTC(),
		}
	}
	defer resp.Body.Close()

	// Check expected status code.
	expectedStatus := 200
	if es, ok := check.Config["expected_status"]; ok {
		switch v := es.(type) {
		case float64:
			expectedStatus = int(v)
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				expectedStatus = parsed
			}
		}
	}

	status := synthetic.ResultStatusOK
	errMsg := ""
	if resp.StatusCode != expectedStatus {
		status = synthetic.ResultStatusFail
		errMsg = fmt.Sprintf("expected status %d, got %d", expectedStatus, resp.StatusCode)
	}

	// Optional body regex match.
	if pattern, ok := check.Config["match_body"].(string); ok && pattern != "" && status == synthetic.ResultStatusOK {
		body := make([]byte, 64*1024)
		n, _ := resp.Body.Read(body)
		re, reErr := CachedRegexp(pattern)
		if reErr != nil {
			status = synthetic.ResultStatusFail
			errMsg = fmt.Sprintf("invalid body regex: %v", reErr)
		} else if !re.Match(body[:n]) {
			status = synthetic.ResultStatusFail
			errMsg = fmt.Sprintf("body did not match pattern: %s", pattern)
		}
	}

	return synthetic.Result{
		CheckID:   check.ID,
		Status:    status,
		LatencyMS: &latencyMS,
		Error:     errMsg,
		Metadata: map[string]any{
			"check_type":  "http",
			"target":      check.Target,
			"status_code": resp.StatusCode,
		},
		CheckedAt: time.Now().UTC(),
	}
}

func executeTCPCheck(check synthetic.Check) synthetic.Result {
	start := time.Now()
	timeout := 3 * time.Second
	if t, ok := check.Config["timeout_seconds"]; ok {
		if ts, ok := t.(float64); ok && ts > 0 {
			timeout = time.Duration(ts) * time.Second
		}
	}

	conn, err := net.DialTimeout("tcp", check.Target, timeout)
	latencyMS := int(time.Since(start).Milliseconds())

	if err != nil {
		return synthetic.Result{
			CheckID:   check.ID,
			Status:    synthetic.ResultStatusFail,
			LatencyMS: &latencyMS,
			Error:     err.Error(),
			Metadata:  map[string]any{"check_type": "tcp", "target": check.Target},
			CheckedAt: time.Now().UTC(),
		}
	}
	_ = conn.Close()

	return synthetic.Result{
		CheckID:   check.ID,
		Status:    synthetic.ResultStatusOK,
		LatencyMS: &latencyMS,
		Metadata:  map[string]any{"check_type": "tcp", "target": check.Target},
		CheckedAt: time.Now().UTC(),
	}
}

func executeDNSCheck(check synthetic.Check) synthetic.Result {
	start := time.Now()

	addrs, err := net.LookupHost(check.Target)
	latencyMS := int(time.Since(start).Milliseconds())

	if err != nil {
		return synthetic.Result{
			CheckID:   check.ID,
			Status:    synthetic.ResultStatusFail,
			LatencyMS: &latencyMS,
			Error:     err.Error(),
			Metadata:  map[string]any{"check_type": "dns", "target": check.Target},
			CheckedAt: time.Now().UTC(),
		}
	}

	status := synthetic.ResultStatusOK
	errMsg := ""

	// Optional expected IP match.
	if expectedIP, ok := check.Config["expected_ip"].(string); ok && expectedIP != "" {
		found := false
		for _, addr := range addrs {
			if addr == expectedIP {
				found = true
				break
			}
		}
		if !found {
			status = synthetic.ResultStatusFail
			errMsg = fmt.Sprintf("expected IP %s not in results: %v", expectedIP, addrs)
		}
	}

	return synthetic.Result{
		CheckID:   check.ID,
		Status:    status,
		LatencyMS: &latencyMS,
		Error:     errMsg,
		Metadata: map[string]any{
			"check_type":     "dns",
			"target":         check.Target,
			"resolved_addrs": addrs,
		},
		CheckedAt: time.Now().UTC(),
	}
}

func executeTLSCheck(check synthetic.Check) synthetic.Result {
	start := time.Now()
	timeout := 10 * time.Second

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", check.Target, &tls.Config{MinVersion: tls.VersionTLS12})
	latencyMS := int(time.Since(start).Milliseconds())

	if err != nil {
		return synthetic.Result{
			CheckID:   check.ID,
			Status:    synthetic.ResultStatusFail,
			LatencyMS: &latencyMS,
			Error:     err.Error(),
			Metadata:  map[string]any{"check_type": "tls_cert", "target": check.Target},
			CheckedAt: time.Now().UTC(),
		}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return synthetic.Result{
			CheckID:   check.ID,
			Status:    synthetic.ResultStatusFail,
			LatencyMS: &latencyMS,
			Error:     "no peer certificates returned",
			Metadata:  map[string]any{"check_type": "tls_cert", "target": check.Target},
			CheckedAt: time.Now().UTC(),
		}
	}

	leaf := certs[0]
	daysUntilExpiry := int(time.Until(leaf.NotAfter).Hours() / 24)

	warnDays := 30
	if wd, ok := check.Config["warn_days"]; ok {
		switch v := wd.(type) {
		case float64:
			warnDays = int(v)
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				warnDays = parsed
			}
		}
	}

	status := synthetic.ResultStatusOK
	errMsg := ""
	if daysUntilExpiry < 0 {
		status = synthetic.ResultStatusFail
		errMsg = fmt.Sprintf("certificate expired %d days ago", -daysUntilExpiry)
	} else if daysUntilExpiry < warnDays {
		status = synthetic.ResultStatusFail
		errMsg = fmt.Sprintf("certificate expires in %d days (warn threshold: %d)", daysUntilExpiry, warnDays)
	}

	return synthetic.Result{
		CheckID:   check.ID,
		Status:    status,
		LatencyMS: &latencyMS,
		Error:     errMsg,
		Metadata: map[string]any{
			"check_type":        "tls_cert",
			"target":            check.Target,
			"subject":           leaf.Subject.CommonName,
			"issuer":            leaf.Issuer.CommonName,
			"not_after":         leaf.NotAfter.Format(time.RFC3339),
			"days_until_expiry": daysUntilExpiry,
		},
		CheckedAt: time.Now().UTC(),
	}
}
