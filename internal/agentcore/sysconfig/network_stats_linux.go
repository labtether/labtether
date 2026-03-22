//go:build linux

package sysconfig

import (
	"os"
	"strconv"
	"strings"
)

// ReadIfaceStats reads RX/TX byte and packet counters from the Linux sysfs
// network statistics interface at /sys/class/net/{name}/statistics/.
// Returns zeroes if any file cannot be read.
func ReadIfaceStats(name string) (rxBytes, txBytes, rxPackets, txPackets uint64) {
	base := "/sys/class/net/" + name + "/statistics/"
	rxBytes = readUint64File(base + "rx_bytes")
	txBytes = readUint64File(base + "tx_bytes")
	rxPackets = readUint64File(base + "rx_packets")
	txPackets = readUint64File(base + "tx_packets")
	return
}

// readUint64File reads a single uint64 value from a sysfs file.
func readUint64File(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
