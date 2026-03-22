//go:build !linux

package remoteaccess

import (
	"context"
	"fmt"
	"io"
	"runtime"
)

// platformStartAudioCapture is the non-Linux fallback — audio capture is not
// yet supported on this platform.
func PlatformStartAudioCapture(_ context.Context, _ string, _ int) (io.Reader, error) {
	return nil, fmt.Errorf("audio capture not supported on %s", runtime.GOOS)
}
