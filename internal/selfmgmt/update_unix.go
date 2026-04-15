//go:build !windows

package selfmgmt

// PrepareForUpdate is a no-op on Unix systems.
// Unix inode semantics allow the installer to overwrite the running binary directly —
// the process holds a file descriptor that keeps the inode alive until it exits.
func PrepareForUpdate(binaryPath string) error {
	return nil
}
