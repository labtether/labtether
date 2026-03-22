//go:build linux

package remoteaccess

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	"github.com/labtether/labtether/internal/securityruntime"
)

// platformStartAudioCapture starts audio capture on Linux using ffmpeg with
// PulseAudio input and Opus encoding, or falls back to GStreamer.
// Returns an io.Reader that produces the raw Opus/OGG audio stream.
func PlatformStartAudioCapture(ctx context.Context, sessionID string, bitrate int) (io.Reader, error) {
	bitrateStr := strconv.Itoa(bitrate / 1000) // ffmpeg uses k notation via -b:a

	// Try ffmpeg first: PulseAudio input → Opus output → stdout.
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		cmd, err := securityruntime.NewCommandContext(ctx, "ffmpeg",
			"-hide_banner", "-loglevel", "error",
			"-f", "pulse", "-i", "default",
			"-c:a", "libopus", "-b:a", bitrateStr+"k",
			"-f", "opus",
			"pipe:1",
		)
		if err != nil {
			return nil, fmt.Errorf("ffmpeg command creation failed: %w", err)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("ffmpeg stdout pipe failed: %w", err)
		}
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("ffmpeg start failed: %w", err)
		}

		// Ensure the process is cleaned up when the context is cancelled.
		go func() {
			_ = cmd.Wait()
		}()

		return stdout, nil
	}

	// Fallback: GStreamer with pulsesrc → opusenc → oggmux → stdout.
	if _, err := exec.LookPath("gst-launch-1.0"); err == nil {
		cmd, err := securityruntime.NewCommandContext(ctx, "gst-launch-1.0",
			"pulsesrc", "!",
			"audioconvert", "!",
			"opusenc", fmt.Sprintf("bitrate=%d", bitrate), "!",
			"oggmux", "!",
			"fdsink", "fd=1",
		)
		if err != nil {
			return nil, fmt.Errorf("gst-launch command creation failed: %w", err)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("gst-launch stdout pipe failed: %w", err)
		}
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("gst-launch start failed: %w", err)
		}

		go func() {
			_ = cmd.Wait()
		}()

		return stdout, nil
	}

	return nil, fmt.Errorf("audio capture requires ffmpeg or gst-launch-1.0 with PulseAudio support")
}
