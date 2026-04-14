package platform

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeRunner is a mock CommandRunner for testing.
type FakeRunner struct {
	Commands  [][]string
	RunFunc   func(name string, args ...string) ([]byte, error)
	LookPaths map[string]string
}

func NewFakeRunner() *FakeRunner {
	return &FakeRunner{
		LookPaths: make(map[string]string),
	}
}

func (f *FakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.Commands = append(f.Commands, append([]string{name}, args...))
	if f.RunFunc != nil {
		return f.RunFunc(name, args...)
	}
	return nil, nil
}

func (f *FakeRunner) LookPath(name string) (string, error) {
	if path, ok := f.LookPaths[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("%s not found", name)
}

// --- Detection Tests ---

func TestCopilotAdapter_Detect(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapter := NewCopilotAdapter(runner)
	assert.True(t, adapter.Detect())
}

func TestCopilotAdapter_DetectNotFound(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewCopilotAdapter(runner)
	assert.False(t, adapter.Detect())
}

func TestClaudeAdapter_Detect(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	adapter := NewClaudeAdapter(runner)
	assert.True(t, adapter.Detect())
}

func TestClaudeAdapter_DetectNotFound(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewClaudeAdapter(runner)
	assert.False(t, adapter.Detect())
}

// --- Install Tests ---

func TestCopilotAdapter_Install(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapter := NewCopilotAdapter(runner)
	err := adapter.Install("gh:owner/repo", ScopeUser)
	require.NoError(t, err)
	assert.Equal(t, []string{"copilot", "plugin", "install", "gh:owner/repo"}, runner.Commands[0])
}

func TestCopilotAdapter_InstallUnsupportedScope(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewCopilotAdapter(runner)
	err := adapter.Install("gh:owner/repo", ScopeProject)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support scope")
}

func TestClaudeAdapter_InstallWithScope(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	adapter := NewClaudeAdapter(runner)
	err := adapter.Install("gh:owner/repo", ScopeProject)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "plugin", "install", "gh:owner/repo", "--scope", "project"}, runner.Commands[0])
}

func TestClaudeAdapter_InstallUserScope(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewClaudeAdapter(runner)
	err := adapter.Install("gh:owner/repo", ScopeUser)
	require.NoError(t, err)
	// user scope should not append --scope flag
	assert.Equal(t, []string{"claude", "plugin", "install", "gh:owner/repo"}, runner.Commands[0])
}

// --- List Tests ---

func TestCopilotAdapter_ListInstalled(t *testing.T) {
	plugins := []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}{
		{Name: "my-plugin", Source: "gh:owner/my-plugin"},
		{Name: "other-plugin", Source: "gh:owner/other-plugin"},
	}
	jsonData, _ := json.Marshal(plugins)

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return jsonData, nil
	}
	adapter := NewCopilotAdapter(runner)
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "my-plugin", result[0].Name)
	assert.Equal(t, "copilot", result[0].Platform)
}

func TestCopilotAdapter_ListInstalledEmpty(t *testing.T) {
	runner := NewFakeRunner()
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte("[]"), nil
	}
	adapter := NewCopilotAdapter(runner)
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestClaudeAdapter_ListInstalledWithScope(t *testing.T) {
	plugins := []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}{
		{Name: "claude-plugin", Source: "gh:owner/claude-plugin"},
	}
	jsonData, _ := json.Marshal(plugins)

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return jsonData, nil
	}
	adapter := NewClaudeAdapter(runner)
	result, err := adapter.ListInstalled(ScopeProject)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "claude-plugin", result[0].Name)
	// Verify scope flag was passed
	assert.Contains(t, runner.Commands[0], "--scope")
	assert.Contains(t, runner.Commands[0], "project")
}

// --- Uninstall Tests ---

func TestCopilotAdapter_Uninstall(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewCopilotAdapter(runner)
	err := adapter.Uninstall("my-plugin", ScopeUser)
	require.NoError(t, err)
	assert.Equal(t, []string{"copilot", "plugin", "uninstall", "my-plugin"}, runner.Commands[0])
}

func TestClaudeAdapter_UninstallWithScope(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewClaudeAdapter(runner)
	err := adapter.Uninstall("my-plugin", ScopeProject)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "plugin", "uninstall", "my-plugin", "--scope", "project"}, runner.Commands[0])
}

// --- DetectAdapters Tests ---

func TestDetectAdapters_BothFound(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	adapters := DetectAdapters(runner)
	assert.Len(t, adapters, 2)
}

func TestDetectAdapters_NoneFound(t *testing.T) {
	runner := NewFakeRunner()
	adapters := DetectAdapters(runner)
	assert.Empty(t, adapters)
}

func TestDetectAdapters_OnlyOne(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapters := DetectAdapters(runner)
	assert.Len(t, adapters, 1)
	assert.Equal(t, "copilot", adapters[0].Name())
}

// --- FilterByTarget Tests ---

func TestFilterByTarget_NoTarget(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	adapters := DetectAdapters(runner)
	filtered, err := FilterByTarget(adapters, "")
	require.NoError(t, err)
	assert.Len(t, filtered, 2)
}

func TestFilterByTarget_SpecificTarget(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	adapters := DetectAdapters(runner)
	filtered, err := FilterByTarget(adapters, "copilot")
	require.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Equal(t, "copilot", filtered[0].Name())
}

func TestFilterByTarget_NotFound(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapters := DetectAdapters(runner)
	_, err := FilterByTarget(adapters, "claude")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Scope Tests ---

func TestValidateScope_CopilotUserScope(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewCopilotAdapter(runner)
	assert.NoError(t, ValidateScope(adapter, ScopeUser))
}

func TestValidateScope_CopilotProjectScope(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewCopilotAdapter(runner)
	assert.Error(t, ValidateScope(adapter, ScopeProject))
}

func TestValidateScope_ClaudeAllScopes(t *testing.T) {
	runner := NewFakeRunner()
	adapter := NewClaudeAdapter(runner)
	assert.NoError(t, ValidateScope(adapter, ScopeUser))
	assert.NoError(t, ValidateScope(adapter, ScopeProject))
	assert.NoError(t, ValidateScope(adapter, ScopeLocal))
}

func TestParseScope(t *testing.T) {
	tests := []struct {
		input string
		want  Scope
		err   bool
	}{
		{"user", ScopeUser, false},
		{"project", ScopeProject, false},
		{"local", ScopeLocal, false},
		{"", ScopeUser, false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseScope(tt.input)
			if tt.err {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
