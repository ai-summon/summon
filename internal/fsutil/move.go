package fsutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

var renameDir = os.Rename

// SetRenameDir overrides the directory rename function used by MoveDir.
// This is primarily intended for tests that simulate cross-filesystem rename errors.
func SetRenameDir(fn func(oldpath, newpath string) error) {
	if fn == nil {
		renameDir = os.Rename
		return
	}
	renameDir = fn
}

// MoveDir moves a directory tree from source to target.
// It prefers os.Rename for speed/atomicity and falls back to copy+delete
// when the move crosses filesystem boundaries.
func MoveDir(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating target parent: %w", err)
	}

	if err := renameDir(source, target); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := copyDir(source, target); err != nil {
		return fmt.Errorf("copying across filesystems: %w", err)
	}
	if err := os.RemoveAll(source); err != nil {
		return fmt.Errorf("removing source after copy: %w", err)
	}
	return nil
}

func copyDir(source, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(target, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, dstPath)
		}

		if d.IsDir() {
			return os.MkdirAll(dstPath, info.Mode().Perm())
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		return copyFile(path, dstPath, info.Mode().Perm())
	})
}

func copyFile(source, target string, perm os.FileMode) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}
