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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
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

	// Group samples by their sorted label set.
	type tsKey struct {
		// We use a canonical string representation as the map key.
		key string
	}
	type tsEntry struct {
		labels  [][2]string // sorted label pairs
		samples []SampleWithLabels
	}

	order := make([]string, 0)
	groups := make(map[string]*tsEntry)

	for _, s := range samples {
		if err := validateSample(s); err != nil {
			return nil, err
		}
		key := labelFingerprint(s.Labels)
		if _, exists := groups[key]; !exists {
			order = append(order, key)
			sorted := sortedLabels(s.Labels)
			groups[key] = &tsEntry{labels: sorted}
		}
		groups[key].samples = append(groups[key].samples, s)
	}

	// Encode as WriteRequest protobuf.
	var buf bytes.Buffer
	for _, key := range order {
		entry := groups[key]
		sort.SliceStable(entry.samples, func(i, j int) bool {
			return entry.samples[i].Timestamp < entry.samples[j].Timestamp
		})
		for i := 1; i < len(entry.samples); i++ {
			if entry.samples[i-1].Timestamp == entry.samples[i].Timestamp {
				return nil, fmt.Errorf("remotewrite: duplicate timestamp for one series")
			}
		}
		tsBytes := encodeTimeSeries(entry.labels, entry.samples)
		// WriteRequest.timeseries = field 1, type LEN
		buf.Write(protowire.AppendTag(nil, 1, protowire.BytesType))
		buf.Write(protowire.AppendBytes(nil, tsBytes))
	}

	compressed := snappy.Encode(nil, buf.Bytes())
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

// ---- protobuf encoding helpers ----

// encodeTimeSeries encodes a single TimeSeries message.
//
//	TimeSeries { labels: repeated Label = 1; samples: repeated Sample = 2 }
func encodeTimeSeries(labels [][2]string, samples []SampleWithLabels) []byte {
	var buf bytes.Buffer

	for _, lp := range labels {
		lb := encodeLabel(lp[0], lp[1])
		buf.Write(protowire.AppendTag(nil, 1, protowire.BytesType))
		buf.Write(protowire.AppendBytes(nil, lb))
	}

	for _, s := range samples {
		sb := encodeSample(s.Value, s.Timestamp)
		buf.Write(protowire.AppendTag(nil, 2, protowire.BytesType))
		buf.Write(protowire.AppendBytes(nil, sb))
	}

	return buf.Bytes()
}

// encodeLabel encodes a Label message.
//
//	Label { name: string = 1; value: string = 2 }
func encodeLabel(name, value string) []byte {
	var buf []byte
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendString(buf, name)
	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendString(buf, value)
	return buf
}

// encodeSample encodes a Sample message.
//
//	Sample { value: double = 1; timestamp: int64 = 2 }
func encodeSample(value float64, timestampMs int64) []byte {
	var buf []byte
	buf = protowire.AppendTag(buf, 1, protowire.Fixed64Type)
	buf = protowire.AppendFixed64(buf, math.Float64bits(value))
	buf = protowire.AppendTag(buf, 2, protowire.VarintType)
	if timestampMs < 0 {
		timestampMs = 0
	}
	buf = protowire.AppendVarint(buf, uint64(timestampMs))
	return buf
}

// sortedLabels returns label pairs in lexicographic order by name.
// __name__ is sorted to the front per Prometheus convention.
func sortedLabels(labels map[string]string) [][2]string {
	pairs := make([][2]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, [2]string{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		// __name__ sorts first.
		if pairs[i][0] == "__name__" {
			return true
		}
		if pairs[j][0] == "__name__" {
			return false
		}
		return pairs[i][0] < pairs[j][0]
	})
	return pairs
}

// labelFingerprint produces a stable string key for a label set.
func labelFingerprint(labels map[string]string) string {
	pairs := sortedLabels(labels)
	var buf bytes.Buffer
	var size [4]byte
	for _, p := range pairs {
		binary.BigEndian.PutUint32(size[:], uint32(len(p[0]))) // #nosec G115 -- validateSample caps label names at 128 bytes.
		buf.Write(size[:])
		buf.WriteString(p[0])
		binary.BigEndian.PutUint32(size[:], uint32(len(p[1]))) // #nosec G115 -- validateSample caps label values at 1024 bytes.
		buf.Write(size[:])
		buf.WriteString(p[1])
	}
	return buf.String()
}
