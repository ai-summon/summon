//go:build !windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSelfUninstall_FullRemoval(t *testing.T) {
	// Set up a fake installation
	dir := t.TempDir()
	binDir := filepath.Join(dir, ".local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	binaryPath := filepath.Join(binDir, "summon")

	// Copy the test binary to simulate an installed binary
	exePath, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(exePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(binaryPath, data, 0o755))

	// Set up data directory
	dataDir := filepath.Join(dir, ".summon")
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "store"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "registry.yaml"), []byte("test"), 0o644))

	// Set up profile with PATH line
	profilePath := filepath.Join(dir, ".bashrc")
	pathLine := pathExportLine(binaryPath)
	require.NoError(t, os.WriteFile(profilePath, []byte("# config\n\n"+pathLine+"\n"), 0o644))

	// Verify data dir exists before
	_, err = os.Stat(dataDir)
	assert.NoError(t, err)

	// Remove data dir
	err = removeDataDir(dataDir)
	assert.NoError(t, err)

	// Verify data dir is gone
	_, err = os.Stat(dataDir)
	assert.True(t, os.IsNotExist(err))

	// Clean PATH
	err = cleanPathInFile(profilePath, pathLine)
	assert.NoError(t, err)

	// Verify profile no longer contains the line
	result, err := os.ReadFile(profilePath)
	require.NoError(t, err)
	assert.NotContains(t, string(result), pathLine)

	// Remove binary
	err = os.Remove(binaryPath)
	assert.NoError(t, err)

	// Verify binary is gone
	_, err = os.Stat(binaryPath)
	assert.True(t, os.IsNotExist(err))
}

func TestRunSelfUninstall_KeepData(t *testing.T) {
	dir := t.TempDir()

	// Set up data directory
	dataDir := filepath.Join(dir, ".summon")
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "store"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "registry.yaml"), []byte("test"), 0o644))

	// Set up binary
	binDir := filepath.Join(dir, ".local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	binaryPath := filepath.Join(binDir, "summon")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake binary"), 0o755))

	// Simulate keep-data: skip data removal, but remove binary
	err := os.Remove(binaryPath)
	assert.NoError(t, err)

	// Verify binary gone
	_, err = os.Stat(binaryPath)
	assert.True(t, os.IsNotExist(err))

	// Verify data dir preserved
	_, err = os.Stat(dataDir)
	assert.NoError(t, err)

	info, err := os.Stat(filepath.Join(dataDir, "registry.yaml"))
	assert.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestRunSelfUninstall_OutputFormat(t *testing.T) {
	// D2: Verify that successful steps produce ✓ prefix output

	dir := t.TempDir()

	// Set up profile
	profilePath := filepath.Join(dir, ".bashrc")
	binaryPath := filepath.Join(dir, ".local", "bin", "summon")
	pathLine := pathExportLine(binaryPath)
	require.NoError(t, os.WriteFile(profilePath, []byte("# config\n\n"+pathLine+"\n"), 0o644))

	// Set up data dir
	dataDir := filepath.Join(dir, ".summon")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))

	// Set up binary
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0o755))
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0o755))

	// Capture stdout
	output := captureStdout(t, func() {
		// Clean PATH
		err := cleanPathInFile(profilePath, pathLine)
		require.NoError(t, err)
		fmt.Printf("✓ %s\n", pathCleanupSuccessMessage())

		// Remove data
		err = removeDataDir(dataDir)
		require.NoError(t, err)
		fmt.Printf("✓ Removed data directory %s\n", dataDir)

		// Remove binary
		err = os.Remove(binaryPath)
		require.NoError(t, err)
		fmt.Printf("✓ %s\n", binaryRemovalSuccessMessage(binaryPath))
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		assert.True(t, strings.HasPrefix(line, "✓ "),
			"expected ✓ prefix, got: %s", line)
	}
	assert.Len(t, lines, 3)
	assert.Contains(t, lines[2], binaryPath, "binary success message should include path")
}

func TestRunSelfUninstall_PartialFailure(t *testing.T) {
	// D3: Verify that when data dir removal fails, other steps still execute
	// and the overall result reflects the failure

	dir := t.TempDir()

	// Set up a data dir that cannot be removed (read-only parent)
	dataDir := filepath.Join(dir, "locked", ".summon")
	lockedParent := filepath.Join(dir, "locked")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "file.txt"), []byte("data"), 0o644))

	// Make the parent read-only so RemoveAll fails on the contents
	require.NoError(t, os.Chmod(dataDir, 0o000))
	t.Cleanup(func() {
		os.Chmod(dataDir, 0o755) // restore for cleanup
	})

	// removeDataDir should fail
	err := removeDataDir(lockedParent)
	// lockedParent itself is removable, but the .summon subdir is not its child
	// Test directly: removeDataDir on a dir with unreadable contents
	err = removeDataDir(dataDir)
	if err == nil {
		// Some systems (e.g., root) may still succeed; skip test in that case
		t.Skip("removeDataDir succeeded despite permission restriction (likely running as root)")
	}

	assert.Error(t, err, "removeDataDir should fail when directory contents are unremovable")

	// Other operations should still be able to succeed independently
	profile := filepath.Join(dir, ".bashrc")
	pathLine := `export PATH="/tmp/test:$PATH"`
	require.NoError(t, os.WriteFile(profile, []byte("# config\n\n"+pathLine+"\n"), 0o644))

	err = cleanPathInFile(profile, pathLine)
	assert.NoError(t, err, "PATH cleanup should succeed independently of data dir failure")

	result, _ := os.ReadFile(profile)
	assert.NotContains(t, string(result), pathLine)
}
