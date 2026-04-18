//go:build !windows

package selfmgmt

import (
	"fmt"
	"io"
	"os"
)

// removeBinary removes the summon binary on Unix systems.
// Unix inode semantics allow deleting a running executable directly.
func removeBinary(binaryPath string, fs FileSystem, w io.Writer) error {
	if err := fs.Remove(binaryPath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied removing %s\ntry running with elevated permissions or manually remove the file", binaryPath)
		}
		return fmt.Errorf("failed to remove %s: %w", binaryPath, err)
	}
	_, _ = fmt.Fprintf(w, "removed %s\n", binaryPath)
	return nil
}
