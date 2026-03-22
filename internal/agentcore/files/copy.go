package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/labtether/labtether/internal/agentmgr"
)

// HandleFileCopy handles a file copy request from the hub.
func (fm *Manager) HandleFileCopy(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileCopyData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid copy request: %v", err)
		return
	}

	srcPath, err := fm.ValidatePath(req.SrcPath)
	if err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	dstPath, err := fm.ValidatePath(req.DstPath)
	if err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	if srcPath == dstPath {
		fm.SendFileResult(transport, req.RequestID, false, "source and destination are identical")
		return
	}

	if _, err := os.Stat(srcPath); err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}
	if _, err := os.Stat(dstPath); err == nil {
		fm.SendFileResult(transport, req.RequestID, false, "destination already exists")
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	if err := CopyPathRecursive(srcPath, dstPath); err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	fm.SendFileResult(transport, req.RequestID, true, "")
}

// CopyPathRecursive copies a file or directory tree from srcPath to dstPath.
// Symlinks are rejected for safety.
func CopyPathRecursive(srcPath, dstPath string) error {
	srcInfo, err := os.Lstat(srcPath)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("copying symlinks is not supported")
	}

	if srcInfo.IsDir() {
		if PathWithinBaseDir(srcPath, dstPath) && srcPath != dstPath {
			return fmt.Errorf("destination cannot be inside source directory")
		}
		if err := os.MkdirAll(dstPath, srcInfo.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(srcPath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := CopyPathRecursive(filepath.Join(srcPath, entry.Name()), filepath.Join(dstPath, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("copying special files is not supported")
	}

	return copyRegularFile(srcPath, dstPath, srcInfo.Mode().Perm())
}

func copyRegularFile(srcPath, dstPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return err
	}

	srcFile, err := os.Open(srcPath) // #nosec G304 -- Agent file copy intentionally targets operator-requested local paths after authz.
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_TRUNC, mode) // #nosec G304 -- Agent file copy intentionally targets operator-requested local paths after authz.
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}
