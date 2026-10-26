package remotewrite

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

type writeSeries struct {
	labels  [][2]string
	samples []SampleWithLabels
	size    int
}

func labelSize(name, value string) int {
	return 2 + protowire.SizeBytes(len(name)) + protowire.SizeBytes(len(value))
}

// Samples have two one-byte tags, a fixed64 value, and a positive timestamp
// already checked by validateSample.
func sampleSize(timestamp int64) int {
	return 10 + protowire.SizeVarint(uint64(timestamp))
}

func timeSeriesSize(labels [][2]string, samples []SampleWithLabels) int {
	size := 0
	for _, pair := range labels {
		size += 1 + protowire.SizeBytes(labelSize(pair[0], pair[1]))
	}
	for _, sample := range samples {
		size += 1 + protowire.SizeBytes(sampleSize(sample.Timestamp))
	}
	return size
}

// appendTimeSeries writes nested Label and Sample messages directly into the
// request buffer. No per-label, per-sample, or per-series byte copies are needed.
func appendTimeSeries(buf []byte, labels [][2]string, samples []SampleWithLabels) []byte {
	for _, pair := range labels {
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(labelSize(pair[0], pair[1])))
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendString(buf, pair[0])
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendString(buf, pair[1])
	}
	for _, sample := range samples {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(sampleSize(sample.Timestamp)))
		buf = protowire.AppendTag(buf, 1, protowire.Fixed64Type)
		buf = protowire.AppendFixed64(buf, math.Float64bits(sample.Value))
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(sample.Timestamp))
	}
	return buf
}

// sortedLabels puts __name__ first and the remaining names in lexical order.
func sortedLabels(labels map[string]string) [][2]string {
	pairs := make([][2]string, 0, len(labels))
	for key, value := range labels {
		pairs = append(pairs, [2]string{key, value})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] == "__name__" {
			return pairs[j][0] != "__name__"
		}
		return pairs[j][0] != "__name__" && pairs[i][0] < pairs[j][0]
	})
	return pairs
}

func labelFingerprint(labels map[string]string) string {
	return sortedLabelFingerprint(sortedLabels(labels))
}

func sortedLabelFingerprint(pairs [][2]string) string {
	length := 0
	for _, pair := range pairs {
		length += 8 + len(pair[0]) + len(pair[1])
	}
	var buf strings.Builder
	buf.Grow(length)
	var size [4]byte
	for _, pair := range pairs {
		binary.BigEndian.PutUint32(size[:], uint32(len(pair[0]))) // #nosec G115 -- validateSample bounds names to 128 bytes.
		buf.Write(size[:])
		buf.WriteString(pair[0])
		binary.BigEndian.PutUint32(size[:], uint32(len(pair[1]))) // #nosec G115 -- validateSample bounds values to 1024 bytes.
		buf.Write(size[:])
		buf.WriteString(pair[1])
	}
	return buf.String()
}
