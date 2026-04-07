// Package fsutil provides cross-platform filesystem helpers used by the
// store and installer packages. On Unix systems directory links are
// symlinks; on Windows they are NTFS junctions (no admin privileges required).
package fsutil

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// CreateDirLink creates a directory link from target → source.
// On Unix this is a symlink; on Windows it creates an NTFS junction via
// "mklink /J" so it works without elevated privileges.
func CreateDirLink(source, target string) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "mklink", "/J", target, source)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("creating junction: %s: %w", string(out), err)
		}
		return nil
	}
	return os.Symlink(source, target)
}

// RemoveLink removes a symlink or junction at path.
// It does not follow the link — only the link entry itself is deleted.
func RemoveLink(path string) error {
	return os.Remove(path)
}

// IsLink reports whether path is a symbolic link or (on Windows) an NTFS
// junction. Go 1.23+ no longer sets ModeSymlink for junctions, so we fall
// back to os.Readlink which succeeds for both symlinks and junctions.
func IsLink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if runtime.GOOS == "windows" {
		_, err := os.Readlink(path)
		return err == nil
	}
	return false
}

// LinkTarget returns the destination path that the symlink at path points to.
func LinkTarget(path string) (string, error) {
	return os.Readlink(path)
}
