//go:build darwin

package sysconfig

import (
	"bufio"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const darwinNetstatTimeout = 2 * time.Second

var runNetstatInterface = func(name string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), darwinNetstatTimeout)
	defer cancel()
	return securityruntime.CommandContextOutput(ctx, "netstat", "-bI", name)
}

// ReadIfaceStats reads per-interface byte and packet counters on macOS by
// parsing `netstat -bI <iface>` output. Returns zeroes if stats are unavailable.
func ReadIfaceStats(name string) (rxBytes, txBytes, rxPackets, txPackets uint64) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, 0, 0, 0
	}

	out, err := runNetstatInterface(name)
	if err != nil || len(out) == 0 {
		return 0, 0, 0, 0
	}

	rxBytes, txBytes, rxPackets, txPackets, ok := ParseDarwinNetstatOutput(name, string(out))
	if !ok {
		return 0, 0, 0, 0
	}
	return rxBytes, txBytes, rxPackets, txPackets
}

func ParseDarwinNetstatOutput(name, raw string) (rxBytes, txBytes, rxPackets, txPackets uint64, ok bool) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var header []string
	var nameIdx, rxBytesIdx, txBytesIdx, rxPacketsIdx, txPacketsIdx int

	bestSum := uint64(0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if header == nil {
			header = fields
			nameIdx = indexOf(header, "Name")
			rxBytesIdx = indexOf(header, "Ibytes")
			txBytesIdx = indexOf(header, "Obytes")
			rxPacketsIdx = indexOf(header, "Ipkts")
			txPacketsIdx = indexOf(header, "Opkts")
			if nameIdx < 0 || rxBytesIdx < 0 || txBytesIdx < 0 || rxPacketsIdx < 0 || txPacketsIdx < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}

		requiredMax := maxInt(maxInt(nameIdx, rxBytesIdx), maxInt(txBytesIdx, maxInt(rxPacketsIdx, txPacketsIdx)))
		if len(fields) <= requiredMax {
			continue
		}
		if fields[nameIdx] != name {
			continue
		}

		currRXBytes, okRXBytes := parseCounterField(fields[rxBytesIdx])
		currTXBytes, okTXBytes := parseCounterField(fields[txBytesIdx])
		currRXPackets, okRXPackets := parseCounterField(fields[rxPacketsIdx])
		currTXPackets, okTXPackets := parseCounterField(fields[txPacketsIdx])
		if !(okRXBytes && okTXBytes && okRXPackets && okTXPackets) {
			continue
		}

		sum := currRXBytes + currTXBytes
		if !ok || sum >= bestSum {
			rxBytes = currRXBytes
			txBytes = currTXBytes
			rxPackets = currRXPackets
			txPackets = currTXPackets
			bestSum = sum
			ok = true
		}
	}

	return rxBytes, txBytes, rxPackets, txPackets, ok
}

func parseCounterField(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func indexOf(fields []string, target string) int {
	for i, field := range fields {
		if field == target {
			return i
		}
	}
	return -1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
