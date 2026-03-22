//go:build !linux && !darwin && !freebsd

package system

import (
	"fmt"

	"github.com/labtether/labtether/internal/agentmgr"
)

// StatfsMountPoint is not supported on this platform; it always returns an error.
// collectMountsLinux is never called on these platforms anyway (only on linux).
func StatfsMountPoint(device, mountPoint, fsType string) (agentmgr.MountInfo, error) {
	return agentmgr.MountInfo{}, fmt.Errorf("statfs not supported on this platform")
}
