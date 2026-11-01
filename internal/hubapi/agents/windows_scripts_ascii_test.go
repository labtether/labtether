package agents

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Windows PowerShell 5.1 reads a .ps1 without a byte order mark in the
// system code page. One em dash in uninstall-agent.ps1 ended a string early,
// so uninstall never removed the Hub SSH key or the install folder.
func TestDeployPowerShellScriptsAreASCII(t *testing.T) {
	root := filepath.Join("..", "..", "..", "deploy")
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".ps1") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		for index, value := range data {
			if value > 0x7f {
				line := bytes.Count(data[:index], []byte("\n")) + 1
				t.Errorf("%s line %d has a non-ASCII byte; Windows PowerShell 5.1 misreads it", path, line)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no .ps1 files found under deploy/")
	}
}
