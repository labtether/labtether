//go:build !linux && !darwin

package sysconfig

// ReadIfaceStats returns zeroes on non-Linux platforms where sysfs is unavailable.
func ReadIfaceStats(_ string) (rxBytes, txBytes, rxPackets, txPackets uint64) {
	return 0, 0, 0, 0
}
