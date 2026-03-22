//go:build windows

package sysconfig

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

func PlatformListDisplays() ([]agentmgr.DisplayInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := securityruntime.CommandContextOutput(ctx, "powershell", "-NoProfile", "-Command",
		`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Screen]::AllScreens | ForEach-Object { "{0}|{1}|{2}|{3}|{4}|{5}" -f $_.DeviceName, $_.Bounds.Width, $_.Bounds.Height, $_.Primary, $_.Bounds.X, $_.Bounds.Y }`)
	if err != nil {
		log.Printf("display: powershell display query failed: %v", err)
		return nil, err
	}

	displays := ParsePowerShellScreenDisplays(string(out))
	return displays, nil
}
