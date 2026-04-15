//go:build windows

package selfmgmt

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// removeBinary removes the summon binary on Windows.
// Windows NTFS enforces exclusive access on running executables, so direct
// deletion fails when the binary is the running process. As a fallback, the
// binary is renamed (NTFS allows renaming a running executable) so that it
// disappears from its original location. A best-effort delete is attempted
// on the renamed file; if it's still locked it will be cleaned up after the
// process exits or on next boot.
func removeBinary(binaryPath string, fs FileSystem, w io.Writer) error {
	if err := fs.Remove(binaryPath); err == nil {
		fmt.Fprintf(w, "removed %s\n", binaryPath)
		return nil
	}

	// Direct removal failed — likely a running executable.
	// Rename it out of the way; NTFS allows renaming a running executable.
	renamedPath := strings.TrimSuffix(binaryPath, ".exe") + ".uninstalling.exe"
	if err := os.Rename(binaryPath, renamedPath); err != nil {
		return fmt.Errorf("permission denied removing %s\nTry running with elevated permissions or manually remove the file.", binaryPath)
	}

	os.Remove(renamedPath) // best-effort cleanup
	fmt.Fprintf(w, "removed %s\n", binaryPath)
	return nil
}
