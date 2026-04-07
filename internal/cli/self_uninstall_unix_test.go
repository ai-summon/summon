//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPathUnix_LineFoundAndRemoved(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, ".bashrc")
	binaryPath := filepath.Join(dir, ".local", "bin", "summon")

	binDir := filepath.Dir(binaryPath)
	pathLine := `export PATH="` + binDir + `:$PATH"`
	original := "# existing config\nalias ll='ls -la'\n\n" + pathLine + "\n"
	expected := "# existing config\nalias ll='ls -la'\n\n"

	require.NoError(t, os.WriteFile(profilePath, []byte(original), 0o644))

	// Override resolveProfilePath by directly calling cleanPathInFile
	err := cleanPathInFile(profilePath, pathLine)
	require.NoError(t, err)

	result, err := os.ReadFile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, expected, string(result))
}

func TestCleanPathUnix_LineNotFound(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, ".bashrc")
	original := "# existing config\nalias ll='ls -la'\n"

	require.NoError(t, os.WriteFile(profilePath, []byte(original), 0o644))

	err := cleanPathInFile(profilePath, `export PATH="/nonexistent:$PATH"`)
	require.NoError(t, err)

	result, err := os.ReadFile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, original, string(result))
}

func TestCleanPathUnix_FileMissing(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, ".bashrc-nonexistent")

	err := cleanPathInFile(profilePath, `export PATH="/foo:$PATH"`)
	assert.NoError(t, err) // should be no-op, not error
}

func TestCleanPathUnix_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, ".bashrc")
	pathLine := `export PATH="/home/user/.local/bin:$PATH"`
	original := "# config\n" + pathLine + "\n# more\n" + pathLine + "\n"
	expected := "# config\n# more\n"

	require.NoError(t, os.WriteFile(profilePath, []byte(original), 0o644))

	err := cleanPathInFile(profilePath, pathLine)
	require.NoError(t, err)

	result, err := os.ReadFile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, expected, string(result))
}

func TestCleanPathUnix_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, ".bashrc")
	pathLine := `export PATH="/home/user/.local/bin:$PATH"`
	original := "# config\n\n" + pathLine + "\n"

	require.NoError(t, os.WriteFile(profilePath, []byte(original), 0o600))

	err := cleanPathInFile(profilePath, pathLine)
	require.NoError(t, err)

	info, err := os.Stat(profilePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPathExportLine(t *testing.T) {
	result := pathExportLine("/home/user/.local/bin/summon")
	assert.Equal(t, `export PATH="/home/user/.local/bin:$PATH"`, result)
}

func TestResolveProfilePath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		shell    string
		expected string
	}{
		{"/bin/zsh", filepath.Join(home, ".zprofile")},
		{"/bin/bash", filepath.Join(home, ".bashrc")},
		{"/bin/fish", filepath.Join(home, ".profile")},
		{"", filepath.Join(home, ".profile")},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			result, err := resolveProfilePath()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// cleanPathInFile is a testable helper extracted from cleanPath.
// Since cleanPath calls resolveProfilePath which depends on env vars,
// tests use this helper to test the core logic directly.
func cleanPathInFile(profilePath, line string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content := string(data)
	if !containsLine(content, line) {
		return nil
	}

	content = removeLine(content, line)

	dir := filepath.Dir(profilePath)
	tmp, err := os.CreateTemp(dir, ".summon-uninstall-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	info, err := os.Stat(profilePath)
	if err == nil {
		os.Chmod(tmpName, info.Mode())
	}

	return os.Rename(tmpName, profilePath)
}
