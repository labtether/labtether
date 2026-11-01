package resources

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	fileSpoolPrefix   = "labtether-"
	staleFileSpoolAge = time.Hour
)

// fileSpoolSweeps runs the leftover-file sweep once per spool directory per
// process, before that process creates its first spool file there.
var fileSpoolSweeps sync.Map // directory -> *sync.Once

// createFileSpool creates a private staging file for a transfer or copy of up
// to 512 MiB. It goes in SpoolDir on the data volume when one is set: the
// container's /tmp is a memory-backed tmpfs that counts against the Hub's
// memory limit, so staging large files there can get the Hub killed.
func (d *Deps) createFileSpool(pattern string) (*os.File, error) {
	dir := ""
	if d != nil {
		dir = strings.TrimSpace(d.SpoolDir)
	}
	if dir != "" {
		err := os.MkdirAll(dir, 0o700)
		if err == nil {
			sweepStaleFileSpools(dir)
			return os.CreateTemp(dir, fileSpoolPrefix+pattern)
		}
		securityruntime.Logf("file: spool directory unavailable, using the system temp directory: %v", err)
	}
	return os.CreateTemp("", fileSpoolPrefix+pattern)
}

// sweepStaleFileSpools removes staging files left behind when an earlier Hub
// process stopped partway through a transfer. Spool files are always removed
// when a transfer ends, so anything old in this directory is abandoned.
func sweepStaleFileSpools(dir string) {
	once, _ := fileSpoolSweeps.LoadOrStore(dir, &sync.Once{})
	once.(*sync.Once).Do(func() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		cutoff := time.Now().Add(-staleFileSpoolAge)
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), fileSpoolPrefix) {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	})
}
