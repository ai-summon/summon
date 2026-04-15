package selfmgmt

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// FileSystem abstracts filesystem operations for testability.
type FileSystem interface {
	RemoveAll(path string) error
	Remove(path string) error
	Stat(path string) (os.FileInfo, error)
}

type osFileSystem struct{}

func (o *osFileSystem) RemoveAll(path string) error { return os.RemoveAll(path) }
func (o *osFileSystem) Remove(path string) error    { return os.Remove(path) }
func (o *osFileSystem) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

// systemWidePrefixes are directories that typically indicate a system-managed installation.
var systemWidePrefixes = []string{
	"/usr/bin/",
	"/usr/local/bin/",
	"/usr/sbin/",
	"/opt/",
}

// isSystemWidePath returns true if the binary is in a system-managed location.
func isSystemWidePath(binaryPath string) bool {
	for _, prefix := range systemWidePrefixes {
		if strings.HasPrefix(binaryPath, prefix) {
			return true
		}
	}
	return false
}

// Uninstall removes the summon config directory and binary.
// Config directory is removed first (FR-005), then the binary.
// Missing config directory is silently skipped (FR-006).
func Uninstall(paths SummonPaths, w io.Writer) error {
	return UninstallWith(paths, w, &osFileSystem{})
}

// UninstallWith removes the summon config directory and binary using the
// provided filesystem abstraction (for testability).
func UninstallWith(paths SummonPaths, w io.Writer, fs FileSystem) error {
	// Warn if binary is in a system-managed location
	if isSystemWidePath(paths.BinaryPath) {
		fmt.Fprintf(w, "warning: %s appears to be in a system-managed location\n", paths.BinaryPath)
		fmt.Fprintln(w, "The binary may be managed by a package manager.")
	}

	// Remove config directory first (FR-005)
	_, err := fs.Stat(paths.ConfigDir)
	if err == nil {
		if err := fs.RemoveAll(paths.ConfigDir); err != nil {
			return fmt.Errorf("permission denied removing %s\nTry running with elevated permissions or manually remove the file.", paths.ConfigDir)
		}
		fmt.Fprintf(w, "removed %s\n", paths.ConfigDir)
	}
	// Missing config dir is silently skipped (FR-006)

	// Remove binary (platform-specific: on Windows, handles locked running executables)
	if err := removeBinary(paths.BinaryPath, fs, w); err != nil {
		return err
	}

	return nil
}
