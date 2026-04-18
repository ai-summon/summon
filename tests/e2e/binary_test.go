package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBinary returns the path to the compiled summon binary.
// It compiles the binary once per test run using t.TempDir().
var testBinaryPath string

func buildBinary(t *testing.T) string {
	t.Helper()
	if testBinaryPath != "" {
		if _, err := os.Stat(testBinaryPath); err == nil {
			return testBinaryPath
		}
	}
	dir, err := os.MkdirTemp("", "summon-e2e-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	binary := filepath.Join(dir, "summon")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	modRoot, absErr := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, absErr)
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/summon")
	cmd.Dir = modRoot
	out, buildErr := cmd.CombinedOutput()
	require.NoError(t, buildErr, "failed to build binary: %s", string(out))
	testBinaryPath = binary
	return binary
}

func TestBinary_Help(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary, "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)

	stdout := string(out)
	assert.Contains(t, stdout, "summon")
	assert.Contains(t, stdout, "plugin dependency manager")
	assert.Contains(t, stdout, "install")
	assert.Contains(t, stdout, "validate")
}

func TestBinary_List(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary, "list")
	out, err := cmd.CombinedOutput()

	// list may fail if no CLIs detected, but should produce output
	stdout := string(out)
	_ = err // exit code may be non-zero if no CLIs
	// Should at least not panic
	assert.NotContains(t, stdout, "panic")
}

func TestBinary_ValidateJSON_NoManifest(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(binary, "validate", "--json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	// Should fail (no summon.yaml) but not panic
	assert.Error(t, err)
	stdout := string(out)
	assert.NotContains(t, stdout, "panic")
}

func TestBinary_ValidateJSON_ValidManifest(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	manifestData := `dependencies:
  - test-plugin
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	cmd := exec.Command(binary, "validate", "--json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)

	// Output should be valid JSON
	stdout := string(out)
	assert.True(t, json.Valid([]byte(stdout)), "validate --json output should be valid JSON: %s", stdout)

	// Parse and verify structure
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.Contains(t, result, "results")
	assert.Contains(t, result, "summary")
}

func TestBinary_NonTTY_NoANSI(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	manifestData := `dependencies:
  - test-plugin
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	// When stdout is captured by os/exec, it's inherently non-TTY.
	// lipgloss should auto-detect this and strip ANSI.
	cmd := exec.Command(binary, "validate")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)

	stdout := string(out)
	// Verify no ANSI escape sequences in non-TTY output
	assert.False(t, strings.Contains(stdout, "\x1b["),
		"non-TTY output should not contain ANSI escape sequences, got: %q", stdout)
}
