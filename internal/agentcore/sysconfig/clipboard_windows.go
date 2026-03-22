//go:build windows

package sysconfig

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/securityruntime"
)

// PlatformClipboardRead reads the Windows clipboard using PowerShell Get-Clipboard.
func PlatformClipboardRead(format string) (text string, imgBase64 string, err error) {
	if format == "image" || format == "image/png" {
		return "", "", fmt.Errorf("image clipboard read not yet supported on Windows")
	}

	out, err := securityruntime.CommandOutput("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	if err != nil {
		return "", "", fmt.Errorf("Get-Clipboard failed: %w", err)
	}
	return strings.TrimRight(string(out), "\r\n"), "", nil
}

// PlatformClipboardWriteText writes text to the Windows clipboard using PowerShell Set-Clipboard.
func PlatformClipboardWriteText(text string) error {
	// Use -Command with pipeline to avoid quoting issues.
	cmd, err := securityruntime.NewCommand("powershell", "-NoProfile", "-Command", "Set-Clipboard -Value $input")
	if err != nil {
		return fmt.Errorf("failed to create PowerShell command: %w", err)
	}
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Set-Clipboard failed: %w", err)
	}
	return nil
}

// PlatformClipboardWriteImage writes an image to the Windows clipboard.
// Not yet supported — returns an error.
func PlatformClipboardWriteImage(base64Data string) error {
	return fmt.Errorf("image clipboard write not yet supported on Windows")
}
