package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveExpectedBinaryPath_Default(t *testing.T) {
	// Ensure env var is not set
	t.Setenv("SUMMON_INSTALL_PATH", "")

	p, err := resolveExpectedBinaryPath()
	require.NoError(t, err)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		assert.Equal(t, filepath.Join(home, ".summon", "bin", "summon.exe"), p)
	} else {
		assert.Equal(t, filepath.Join(home, ".local", "bin", "summon"), p)
	}
}

func TestResolveExpectedBinaryPath_CustomEnv(t *testing.T) {
	tmp := t.TempDir()
	customPath := filepath.Join(tmp, "my-summon")
	t.Setenv("SUMMON_INSTALL_PATH", customPath)

	p, err := resolveExpectedBinaryPath()
	require.NoError(t, err)
	assert.Equal(t, customPath, p)
}

func TestResolveExpectedBinaryPath_TildeExpansion(t *testing.T) {
	t.Setenv("SUMMON_INSTALL_PATH", "~/custom/bin/summon")

	p, err := resolveExpectedBinaryPath()
	require.NoError(t, err)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "custom", "bin", "summon"), p)
}

func TestDetectExternalInstall_Mismatch(t *testing.T) {
	// The running test binary is definitely not at ~/.local/bin/summon
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	fakePath := filepath.Join(home, ".local", "bin", "summon-nonexistent")

	err = detectExternalInstall(fakePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "installed externally")
}

func TestDetectExternalInstall_Match(t *testing.T) {
	// Test that matching paths succeed
	exePath, err := os.Executable()
	require.NoError(t, err)
	exePath, err = filepath.EvalSymlinks(exePath)
	require.NoError(t, err)

	err = detectExternalInstall(exePath)
	assert.NoError(t, err)
}

func TestRemoveDataDir_Exists(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".summon")
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "store"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "registry.yaml"), []byte("test"), 0o644))

	err := removeDataDir(dataDir)
	assert.NoError(t, err)

	_, statErr := os.Stat(dataDir)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRemoveDataDir_NotExists(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".summon-nonexistent")

	err := removeDataDir(dataDir)
	assert.NoError(t, err) // should not error
}

func TestConfirmUninstall_YesFlagSkipsPrompt(t *testing.T) {
	assert.True(t, confirmUninstall(true))
}

func TestConfirmUninstall_NonTerminalReturnsFalse(t *testing.T) {
	// When stdin is not a terminal (e.g., pipe in tests), confirmUninstall
	// should return false without --yes.
	assert.False(t, confirmUninstall(false))
}

func TestPromptConfirm_UserDeclinesN(t *testing.T) {
	r := strings.NewReader("n\n")
	assert.False(t, promptConfirm(r))
}

func TestPromptConfirm_UserDeclinesEmpty(t *testing.T) {
	r := strings.NewReader("\n")
	assert.False(t, promptConfirm(r))
}

func TestPromptConfirm_UserAcceptsY(t *testing.T) {
	r := strings.NewReader("y\n")
	assert.True(t, promptConfirm(r))
}

func TestPromptConfirm_UserAcceptsYes(t *testing.T) {
	r := strings.NewReader("yes\n")
	assert.True(t, promptConfirm(r))
}

func TestPromptConfirm_EOF(t *testing.T) {
	r := strings.NewReader("")
	assert.False(t, promptConfirm(r))
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		input    string
		expected string
	}{
		{"~/foo/bar", filepath.Join(home, "foo", "bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := expandHome(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
