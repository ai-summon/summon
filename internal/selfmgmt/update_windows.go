//go:build windows

package selfmgmt

import (
	"fmt"
	"os"
	"strings"
)

// PrepareForUpdate renames the running binary to summon.previous.exe on Windows.
// Windows NTFS enforces exclusive access on running executables, so the file cannot
// be deleted or overwritten directly. Renaming succeeds because only the directory
// entry changes. The caller is responsible for cleanup of .previous.exe after
// successful update.
func PrepareForUpdate(binaryPath string) error {
	previousPath := strings.TrimSuffix(binaryPath, ".exe") + ".previous.exe"
	if err := os.Rename(binaryPath, previousPath); err != nil {
		return fmt.Errorf("cannot rename running binary for update: %w", err)
	}
	return nil
}
