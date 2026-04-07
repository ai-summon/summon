package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: E2E tests for summon new command focus on command-line behavior and error handling.
// File operation tests are covered by comprehensive unit tests in internal/cli/new_test.go.
// This is because e2e tests build an isolated binary that doesn't include the template files.

// ========== Phase 3: User Story 1 - Command Structure ==========

func TestNewCmd_HelpDisplays(t *testing.T) {
	binary := buildBinary(t)

	cmd := exec.Command(binary, "new", "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	output := string(out)
	assert.Contains(t, output, "Create a new plugin project")
	assert.Contains(t, output, "--type")
	assert.Contains(t, output, "--vcs")
	assert.Contains(t, output, "--name")
}

func TestNewCmd_InvalidTypeRejectsUnknownType(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pluginDir := filepath.Join(projectDir, "test-plugin")

	cmd := exec.Command(binary, "new", "--type", "invalid-type", pluginDir)
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "invalid plugin type")
}

func TestNewCmd_InvalidVCSRejectsUnknownOption(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pluginDir := filepath.Join(projectDir, "test-plugin")

	cmd := exec.Command(binary, "new", "--vcs", "svn", pluginDir)
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "invalid vcs option")
}

func TestNewCmd_RequiresProjectPath(t *testing.T) {
	binary := buildBinary(t)

	// Call without project path
	cmd := exec.Command(binary, "new")
	_, err := cmd.CombinedOutput()
	assert.Error(t, err)
}

func TestNewCmd_ValidTypesAccepted(t *testing.T) {
	binary := buildBinary(t)
	validTypes := []string{"skill", "agent", "command", "hook", "mcp", "generic"}

	for _, pluginType := range validTypes {
		t.Run(pluginType, func(t *testing.T) {
			projectDir := t.TempDir()
			pluginDir := filepath.Join(projectDir, fmt.Sprintf("test-%s", pluginType))

			cmd := exec.Command(binary, "new", "--type", pluginType, pluginDir)
			out, err := cmd.CombinedOutput()
			output := string(out)

			// Command should not error on valid type
			// (actual file creation may fail due to missing templates in e2e binary,
			// but validation should pass)
			if err != nil {
				// If error, it should be about templates not found, not invalid type
				assert.NotContains(t, output, "invalid plugin type")
			}
		})
	}
}

func TestNewCmd_ValidVCSOptionsAccepted(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pluginDir := filepath.Join(projectDir, "test-plugin")

	cmd := exec.Command(binary, "new", "--vcs", "git", pluginDir)
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Should not error on invalid vcs
	if err != nil {
		assert.NotContains(t, output, "invalid vcs option")
	}
}

func TestNewCmd_ExistingDirectoryRejected(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pluginDir := filepath.Join(projectDir, "test-plugin")

	// Create directory first
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	// Try to create in existing directory
	cmd := exec.Command(binary, "new", pluginDir)
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "already exists")
}
