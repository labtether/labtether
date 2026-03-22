//go:build linux || darwin || freebsd

package system

import (
	"syscall"

	"github.com/labtether/labtether/internal/agentmgr"
)

// StatfsMountPoint uses syscall.Statfs to collect disk space info for a mount point.
func StatfsMountPoint(device, mountPoint, fsType string) (agentmgr.MountInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountPoint, &stat); err != nil {
		return agentmgr.MountInfo{}, err
	}

	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	available := stat.Bavail * blockSize
	used := total - (stat.Bfree * blockSize)

	var usePct float64
	if total > 0 {
		usePct = float64(used) / float64(total) * 100
	}

	return agentmgr.MountInfo{
		Device:     device,
		MountPoint: mountPoint,
		FSType:     fsType,
		Total:      total,
		Used:       used,
		Available:  available,
		UsePct:     usePct,
	}, nil
}
