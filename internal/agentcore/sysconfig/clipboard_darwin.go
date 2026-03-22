//go:build darwin

package sysconfig

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
)

// PlatformClipboardRead reads the macOS clipboard using pbpaste.
func PlatformClipboardRead(format string) (text string, imgBase64 string, err error) {
	if format == "image" || format == "image/png" {
		return "", "", fmt.Errorf("image clipboard read not yet supported on macOS")
	}

	out, err := securityruntime.CommandOutput("pbpaste")
	if err != nil {
		return "", "", fmt.Errorf("pbpaste failed: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), "", nil
}

// PlatformClipboardWriteText writes text to the macOS clipboard using pbcopy.
func PlatformClipboardWriteText(text string) error {
	cmd, err := securityruntime.NewCommand("pbcopy")
	if err != nil {
		return fmt.Errorf("failed to create pbcopy command: %w", err)
	}
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pbcopy failed: %w", err)
	}
	return nil
}

// PlatformClipboardWriteImage writes an image to the macOS clipboard.
// Not yet supported — returns an error.
func PlatformClipboardWriteImage(base64Data string) error {
	return fmt.Errorf("image clipboard write not yet supported on macOS")
}
