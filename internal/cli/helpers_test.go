package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureStdout replaces os.Stdout with a pipe, runs fn, and returns
// everything written to stdout as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

// setupProjectDir creates a temp directory, chdirs into it, and returns
// the directory path. The original working directory is restored on cleanup.
func setupProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(origDir) })
	return dir
}

// writeRegistryYAML writes a registry.yaml into .summon/ under dir.
func writeRegistryYAML(t *testing.T, dir string, content string) {
	t.Helper()
	writeScopedRegistryYAML(t, dir, "local", content)
}

// createStorePackage creates a minimal package directory inside .summon/store/.
func createStorePackage(t *testing.T, dir, name string) {
	t.Helper()
	createScopedStorePackage(t, dir, "local", name)
}

func writeScopedRegistryYAML(t *testing.T, dir, scope, content string) {
	t.Helper()
	summonDir := filepath.Join(dir, ".summon", scope)
	require.NoError(t, os.MkdirAll(summonDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(summonDir, "registry.yaml"),
		[]byte(content), 0o644,
	))
}

func createScopedStorePackage(t *testing.T, dir, scope, name string) {
	t.Helper()
	pkgDir := filepath.Join(dir, ".summon", scope, "store", name)
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
}
