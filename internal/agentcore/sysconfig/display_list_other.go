//go:build !linux && !darwin && !windows

package sysconfig

import (
	"github.com/labtether/labtether/internal/agentmgr"
)

// PlatformListDisplays returns a single default display for unsupported
// platforms (FreeBSD, etc.) where display enumeration tools are unavailable.
func PlatformListDisplays() ([]agentmgr.DisplayInfo, error) {
	return []agentmgr.DisplayInfo{
		{
			Name:    ":0",
			Width:   1920,
			Height:  1080,
			Primary: true,
		},
	}, nil
}
