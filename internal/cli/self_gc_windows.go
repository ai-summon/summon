//go:build windows

package cli

// RunGC is the entry point for the GC helper process on Windows.
// It waits for the parent process to exit, then deletes the original binary
// and optionally the data directory.
func RunGC(binaryPath, dataDir string) {
	completeWindowsUninstall(binaryPath, dataDir)
}
