//go:build !windows

package cli

// RunGC is a no-op on Unix. The GC pattern is only used on Windows.
func RunGC(_, _ string) {}
