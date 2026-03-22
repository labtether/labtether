//go:build !linux && !darwin && !windows

package sysconfig

import (
	"fmt"
	"runtime"
)

// PlatformClipboardRead is the fallback for unsupported platforms.
func PlatformClipboardRead(format string) (text string, imgBase64 string, err error) {
	return "", "", fmt.Errorf("clipboard read not supported on %s", runtime.GOOS)
}

// PlatformClipboardWriteText is the fallback for unsupported platforms.
func PlatformClipboardWriteText(text string) error {
	return fmt.Errorf("clipboard write not supported on %s", runtime.GOOS)
}

// PlatformClipboardWriteImage is the fallback for unsupported platforms.
func PlatformClipboardWriteImage(base64Data string) error {
	return fmt.Errorf("clipboard image write not supported on %s", runtime.GOOS)
}
