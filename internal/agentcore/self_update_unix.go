//go:build !windows

package agentcore

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func replaceExecutable(executablePath string, binary []byte) error {
	info, err := os.Stat(executablePath)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}

	tmpPath := filepath.Join(filepath.Dir(executablePath), fmt.Sprintf(".%s.update-%d", filepath.Base(executablePath), time.Now().UnixNano()))
	if err := os.WriteFile(tmpPath, binary, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, executablePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
