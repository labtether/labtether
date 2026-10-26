// Package remotewrite implements a Prometheus remote_write client.
//
// It serializes metric samples to the snappy-compressed protobuf format
// defined by the Prometheus remote_write specification (version 0.1.0) and
// pushes them to a remote_write endpoint.  The implementation is compatible
// with Prometheus, Grafana Mimir, Thanos Receive, and Grafana Cloud.
//
// Protobuf encoding is done with the protowire package (already a transitive
// dep via google.golang.org/protobuf) rather than importing the full
// github.com/prometheus/prometheus module, keeping the dependency footprint
// minimal.
//
// Remote_write protobuf schema (field numbers come from
// https://github.com/prometheus/prometheus/blob/main/prompb/remote.proto):
//
//	WriteRequest  { timeseries: repeated TimeSeries  = field 1 }
//	TimeSeries    { labels: repeated Label = 1, samples: repeated Sample = 2 }
//	Label         { name: string = 1, value: string = 2 }
//	Sample        { value: double = 1, timestamp: int64 = 2 }
package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/snappy"
	"github.com/labtether/labtether/internal/securityruntime"
	"google.golang.org/protobuf/encoding/protowire"
)

// SampleWithLabels is a single metric data point with its full label set.
// The __name__ label must be present; all other labels are optional.
// Timestamp is Unix milliseconds (consistent with the Prometheus remote_write spec).
type SampleWithLabels struct {
	Labels    map[string]string // must include "__name__"
	Value     float64
	Timestamp int64 // Unix milliseconds
}

const (
	MaxSamplesPerRequest  = 500
	MaxLabelsPerSample    = 64
	MaxLabelNameBytes     = 128
	MaxLabelValueBytes    = 1024
	MaxSampleBytes        = 24 * 1024
	MaxRequestBodyBytes   = 4 * 1024 * 1024
	maxResponseDrainBytes = 64 * 1024
)

var errRequestBodyTooLarge = errors.New("remotewrite: request body exceeds limit")

type receiverStatusError struct {
	statusCode int
}

func (e *receiverStatusError) Error() string {
	return fmt.Sprintf("remotewrite: receiver returned status %d", e.statusCode)
}

// SerializeWriteRequest encodes samples into a snappy-compressed protobuf body
// suitable for HTTP POST to a Prometheus remote_write endpoint.
//
// Samples are grouped by their label fingerprint so that multiple points with
// identical labels are placed in the same TimeSeries message, which is more
// efficient and required by some remote_write receivers.
//
// Returns nil, nil for an empty input slice.
func SerializeWriteRequest(samples []SampleWithLabels) ([]byte, error) {
	if len(samples) == 0 {
		return nil, nil
	}
	if len(samples) > MaxSamplesPerRequest {
		return nil, fmt.Errorf("remotewrite: sample count exceeds limit")
	}

	// Sort each label set once, retaining those pairs for wire encoding.
	order := make([]*writeSeries, 0)
	groups := make(map[string]*writeSeries)
	for _, sample := range samples {
		if err := validateSample(sample); err != nil {
			return nil, err
		}
		labels := sortedLabels(sample.Labels)
		key := sortedLabelFingerprint(labels)
		entry := groups[key]
		if entry == nil {
			entry = &writeSeries{labels: labels}
			groups[key] = entry
			order = append(order, entry)
		}
		entry.samples = append(entry.samples, sample)
	}

	// Reserve one wire buffer instead of allocating for every nested message.
	totalSize := 0
	for _, entry := range order {
		sort.SliceStable(entry.samples, func(i, j int) bool {
			return entry.samples[i].Timestamp < entry.samples[j].Timestamp
		})
		for i := 1; i < len(entry.samples); i++ {
			if entry.samples[i-1].Timestamp == entry.samples[i].Timestamp {
				return nil, fmt.Errorf("remotewrite: duplicate timestamp for one series")
			}
		}
		entry.size = timeSeriesSize(entry.labels, entry.samples)
		totalSize += 1 + protowire.SizeBytes(entry.size)
	}
	buf := make([]byte, 0, totalSize)
	for _, entry := range order {
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(entry.size))
		buf = appendTimeSeries(buf, entry.labels, entry.samples)
	}

	compressed := snappy.Encode(nil, buf)
	if len(compressed) > MaxRequestBodyBytes {
		return nil, errRequestBodyTooLarge
	}
	return compressed, nil
}

func validateSample(sample SampleWithLabels) error {
	if sample.Timestamp <= 0 {
		return fmt.Errorf("remotewrite: sample timestamp must be positive")
	}
	if len(sample.Labels) == 0 || len(sample.Labels) > MaxLabelsPerSample {
		return fmt.Errorf("remotewrite: sample label count is invalid")
	}
	metricName, hasName := sample.Labels["__name__"]
	if !hasName || !validMetricName(metricName) {
		return fmt.Errorf("remotewrite: sample metric name is invalid")
	}
	total := 24
	for name, value := range sample.Labels {
		if !validLabelName(name) || len(name) > MaxLabelNameBytes || !utf8.ValidString(name) {
			return fmt.Errorf("remotewrite: sample label name is invalid")
		}
		if len(value) > MaxLabelValueBytes || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("remotewrite: sample label value exceeds limit")
		}
		total += len(name) + len(value)
	}
	if total > MaxSampleBytes {
		return fmt.Errorf("remotewrite: sample exceeds byte limit")
	}
	return nil
}

func validMetricName(value string) bool {
	if value == "" || len(value) > MaxLabelValueBytes || !utf8.ValidString(value) {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == ':' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func validLabelName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// remoteWriteClient is the HTTP client used by Push. It has a 15-second timeout
// to prevent goroutine leaks on stalled remote_write endpoints. We do not use
// http.DefaultClient because it has no timeout.
var remoteWriteClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		// Never forward Basic Auth or a receiver-specific request to another
		// origin. Operators must configure the final write endpoint explicitly.
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		Proxy:                 nil,
		MaxConnsPerHost:       4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	},
}

var outboundPushSlots = make(chan struct{}, 4)

// Push sends a serialized WriteRequest body to the given remote_write URL.
// It sets the required Content-Type, Content-Encoding, and version headers.
// HTTP 2xx responses are treated as success; any other status is an error.
func Push(ctx context.Context, url string, body []byte, username, password string) error {
	if ctx == nil {
		return fmt.Errorf("remotewrite: request context is required")
	}
	if len(body) == 0 || len(body) > MaxRequestBodyBytes {
		return fmt.Errorf("remotewrite: request body size is invalid")
	}
	select {
	case outboundPushSlots <- struct{}{}:
		defer func() { <-outboundPushSlots }()
	case <-ctx.Done():
		return fmt.Errorf("remotewrite: request canceled before dispatch")
	}
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("remotewrite: endpoint is invalid or disallowed")
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("User-Agent", "labtether-hub/1.0")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := securityruntime.DoOutboundRequest(remoteWriteClient, req)
	if err != nil {
		return fmt.Errorf("remotewrite: request failed")
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, maxResponseDrainBytes)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &receiverStatusError{statusCode: resp.StatusCode}
	}
	return nil
}

// TimeFromMillis converts a Unix millisecond timestamp to a time.Time.
func TimeFromMillis(ms int64) time.Time {
	return time.UnixMilli(ms)
}
