package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setHome sets the home directory env vars for both Unix (HOME) and Windows (USERPROFILE).
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

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
	// Copilot CLI outputs human-readable text, not JSON
	textOutput := `Installed plugins:
  • my-plugin (v1.0.0)
  • other-plugin@my-marketplace`

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(textOutput), nil
	}
	adapter := NewCopilotAdapter(runner)
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "my-plugin", result[0].Name)
	assert.Equal(t, "1.0.0", result[0].Version)
	assert.Equal(t, "copilot", result[0].Platform)
	assert.Equal(t, "user", result[0].Scope)
	assert.Equal(t, "other-plugin", result[1].Name)
	assert.Equal(t, "", result[1].Version)
	assert.Equal(t, "user", result[1].Scope)
}

func TestCopilotAdapter_ListInstalledEmpty(t *testing.T) {
	runner := NewFakeRunner()
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte("No plugins installed.\n"), nil
	}
	adapter := NewCopilotAdapter(runner)
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestClaudeAdapter_ListInstalledWithScope(t *testing.T) {
	// Claude CLI outputs JSON with "id" field format: "name@marketplace"
	// claude plugin list --json returns ALL scopes; filtering is done in code
	jsonOutput := `[{"id":"claude-plugin@my-marketplace","version":"1.0.0","scope":"project","enabled":true}]`

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(jsonOutput), nil
	}
	adapter := NewClaudeAdapter(runner)
	result, err := adapter.ListInstalled(ScopeProject)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "claude-plugin", result[0].Name)
	assert.Equal(t, "claude-plugin@my-marketplace", result[0].Source)
	assert.Equal(t, "project", result[0].Scope)
	assert.Equal(t, "1.0.0", result[0].Version)
	// claude plugin list does not support --scope; verify it's NOT passed
	assert.NotContains(t, runner.Commands[0], "--scope")
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

// --- CLI Error Output Tests ---

func TestCliError_WithOutput(t *testing.T) {
	err := cliError("claude uninstall", []byte("Error: plugin 'foo' not found\n"), fmt.Errorf("exit status 1"))
	assert.Contains(t, err.Error(), "plugin 'foo' not found")
	assert.NotContains(t, err.Error(), "exit status 1")
}

func TestCliError_EmptyOutput(t *testing.T) {
	err := cliError("claude uninstall", nil, fmt.Errorf("exit status 1"))
	assert.Contains(t, err.Error(), "exit status 1")
}

func TestCliError_WhitespaceOnlyOutput(t *testing.T) {
	err := cliError("claude uninstall", []byte("  \n  "), fmt.Errorf("exit status 1"))
	assert.Contains(t, err.Error(), "exit status 1")
}

func TestClaudeAdapter_UninstallErrorIncludesOutput(t *testing.T) {
	runner := NewFakeRunner()
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte("Error: no such plugin 'superpowers'"), fmt.Errorf("exit status 1")
	}
	adapter := NewClaudeAdapter(runner)
	err := adapter.Uninstall("superpowers", ScopeUser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such plugin 'superpowers'")
}

func TestCopilotAdapter_UninstallErrorIncludesOutput(t *testing.T) {
	runner := NewFakeRunner()
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte("plugin not installed"), fmt.Errorf("exit status 1")
	}
	adapter := NewCopilotAdapter(runner)
	err := adapter.Uninstall("superpowers", ScopeUser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin not installed")
}

// --- ProjectPath Filtering Tests ---

func TestClaudeAdapter_ListInstalled_FiltersOtherProjects(t *testing.T) {
	// Plugin at project scope for /other/project should be filtered out
	jsonOutput := `[
		{"id":"user-plugin@mp","scope":"user","enabled":true},
		{"id":"local-plugin@mp","scope":"project","enabled":true,"projectPath":"/current/project"},
		{"id":"other-plugin@mp","scope":"project","enabled":true,"projectPath":"/other/project"}
	]`

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(jsonOutput), nil
	}
	adapter := NewClaudeAdapterWithCwd(runner, "/current/project")
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "user-plugin", result[0].Name)
	assert.Equal(t, "local-plugin", result[1].Name)
}

func TestClaudeAdapter_ListInstalled_IncludesSubdirectory(t *testing.T) {
	// Running from a subdirectory of the project should still include its plugins
	jsonOutput := `[{"id":"my-plugin@mp","scope":"project","enabled":true,"projectPath":"/my/project"}]`

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(jsonOutput), nil
	}
	adapter := NewClaudeAdapterWithCwd(runner, "/my/project/src/cmd")
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "my-plugin", result[0].Name)
}

func TestClaudeAdapter_ListInstalled_NoProjectPath(t *testing.T) {
	// Project-scope plugin without projectPath should be included (backward compat)
	jsonOutput := `[{"id":"legacy@mp","scope":"project","enabled":true}]`

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(jsonOutput), nil
	}
	adapter := NewClaudeAdapterWithCwd(runner, "/any/dir")
	result, err := adapter.ListInstalled(ScopeUser)
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestIsUnderPath(t *testing.T) {
	assert.True(t, isUnderPath("/a/b", "/a/b"))
	assert.True(t, isUnderPath("/a/b/c", "/a/b"))
	assert.False(t, isUnderPath("/a/bc", "/a/b"))
	assert.False(t, isUnderPath("/a/b-dev/c", "/a/b"))
	assert.False(t, isUnderPath("/other", "/a/b"))
}

// --- ListMarketplaces Tests ---

func TestCopilotAdapter_ListMarketplaces_MixedSymbols(t *testing.T) {
	// Actual copilot output uses ◆ for built-in and • for user-registered
	output := "✨ Included with GitHub Copilot:\n" +
		"  ◆ copilot-plugins (GitHub: github/copilot-plugins)\n" +
		"  ◆ awesome-copilot (GitHub: github/awesome-copilot)\n" +
		"\n" +
		"Registered marketplaces:\n" +
		"  • summon-marketplace (GitHub: ai-summon/summon-marketplace)\n"

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(output), nil
	}
	adapter := NewCopilotAdapter(runner)
	marketplaces, err := adapter.ListMarketplaces()
	require.NoError(t, err)
	assert.Len(t, marketplaces, 3)
	assert.Equal(t, "copilot-plugins", marketplaces[0].Name)
	assert.Equal(t, "awesome-copilot", marketplaces[1].Name)
	assert.Equal(t, "summon-marketplace", marketplaces[2].Name)
	assert.Equal(t, "ai-summon/summon-marketplace", marketplaces[2].Source)
}

func TestCopilotAdapter_ListMarketplaces_URLFormat(t *testing.T) {
	// GHE-hosted marketplaces use "URL:" instead of "GitHub:"
	output := "✨ Included with GitHub Copilot:\n" +
		"  ◆ copilot-plugins (GitHub: github/copilot-plugins)\n" +
		"\n" +
		"Registered marketplaces:\n" +
		"  • summon-marketplace (GitHub: ai-summon/summon-marketplace)\n" +
		"  • bmw-ai-marketplace (URL: https://cc-github.bmwgroup.net/ismaeljimenez-martinez/bmw-ai-marketplace)\n"

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(output), nil
	}
	adapter := NewCopilotAdapter(runner)
	marketplaces, err := adapter.ListMarketplaces()
	require.NoError(t, err)
	assert.Len(t, marketplaces, 3)
	assert.Equal(t, "copilot-plugins", marketplaces[0].Name)
	assert.Equal(t, "summon-marketplace", marketplaces[1].Name)
	assert.Equal(t, "bmw-ai-marketplace", marketplaces[2].Name)
	assert.Equal(t, "https://cc-github.bmwgroup.net/ismaeljimenez-martinez/bmw-ai-marketplace", marketplaces[2].Source)
}

func TestClaudeAdapter_ListMarketplaces_GitSource(t *testing.T) {
	// Claude CLI returns "source":"git" + "url":"..." for non-GitHub.com marketplaces
	jsonOutput := `[
		{"name":"bmw-ai-marketplace","source":"git","url":"https://cc-github.bmwgroup.net/ismaeljimenez-martinez/bmw-ai-marketplace.git"},
		{"name":"claude-plugins-official","source":"github","repo":"anthropics/claude-plugins-official"},
		{"name":"summon-marketplace","source":"github","repo":"ai-summon/summon-marketplace"}
	]`

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		return []byte(jsonOutput), nil
	}
	adapter := NewClaudeAdapter(runner)
	marketplaces, err := adapter.ListMarketplaces()
	require.NoError(t, err)
	assert.Len(t, marketplaces, 3)
	assert.Equal(t, "bmw-ai-marketplace", marketplaces[0].Name)
	assert.Equal(t, "https://cc-github.bmwgroup.net/ismaeljimenez-martinez/bmw-ai-marketplace.git", marketplaces[0].Source)
	assert.Equal(t, "anthropics/claude-plugins-official", marketplaces[1].Source)
	assert.Equal(t, "ai-summon/summon-marketplace", marketplaces[2].Source)
}

// --- EnsureMarketplace Tests ---

func TestCopilotAdapter_EnsureMarketplace_AlreadyRegistered(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	// Realistic output: built-in use ◆, user-registered use •
	realOutput := "✨ Included with GitHub Copilot:\n" +
		"  ◆ copilot-plugins (GitHub: github/copilot-plugins)\n" +
		"  ◆ awesome-copilot (GitHub: github/awesome-copilot)\n" +
		"\n" +
		"Registered marketplaces:\n" +
		"  • summon-marketplace (GitHub: ai-summon/summon-marketplace)\n"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(realOutput), nil
			}
		}
		return nil, nil
	}
	adapter := NewCopilotAdapter(runner)
	err := adapter.EnsureMarketplace("summon-marketplace", "ai-summon/summon-marketplace")
	require.NoError(t, err)

	// Should NOT have called marketplace add
	for _, cmd := range runner.Commands {
		for _, a := range cmd {
			assert.NotEqual(t, "add", a, "should not call marketplace add when already registered")
		}
	}

	// Should have called marketplace update
	lastCmd := runner.Commands[len(runner.Commands)-1]
	assert.Contains(t, lastCmd, "update")
	assert.Contains(t, lastCmd, "summon-marketplace")
}

func TestCopilotAdapter_EnsureMarketplace_NotRegistered(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte("No marketplaces registered.\n"), nil
			}
		}
		return nil, nil
	}
	adapter := NewCopilotAdapter(runner)
	err := adapter.EnsureMarketplace("summon-marketplace", "ai-summon/summon-marketplace")
	require.NoError(t, err)

	// Should have called marketplace add
	lastCmd := runner.Commands[len(runner.Commands)-1]
	assert.Contains(t, lastCmd, "add")
	assert.Contains(t, lastCmd, "ai-summon/summon-marketplace")
}

func TestClaudeAdapter_EnsureMarketplace_AlreadyRegistered(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"name":"summon-marketplace","source":"ai-summon/summon-marketplace"}]`), nil
			}
		}
		return nil, nil
	}
	adapter := NewClaudeAdapter(runner)
	err := adapter.EnsureMarketplace("summon-marketplace", "ai-summon/summon-marketplace")
	require.NoError(t, err)

	// Should NOT have called marketplace add
	for _, cmd := range runner.Commands {
		for _, a := range cmd {
			assert.NotEqual(t, "add", a)
		}
	}

	// Should have called marketplace update
	lastCmd := runner.Commands[len(runner.Commands)-1]
	assert.Contains(t, lastCmd, "update")
	assert.Contains(t, lastCmd, "summon-marketplace")
}

func TestClaudeAdapter_EnsureMarketplace_NotRegistered(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte("[]"), nil
			}
		}
		return nil, nil
	}
	adapter := NewClaudeAdapter(runner)
	err := adapter.EnsureMarketplace("summon-marketplace", "ai-summon/summon-marketplace")
	require.NoError(t, err)

	lastCmd := runner.Commands[len(runner.Commands)-1]
	assert.Contains(t, lastCmd, "add")
	assert.Contains(t, lastCmd, "ai-summon/summon-marketplace")
}

func TestCopilotAdapter_EnsureMarketplace_UpdateFailure(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	realOutput := "Registered marketplaces:\n" +
		"  • summon-marketplace (GitHub: ai-summon/summon-marketplace)\n"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(realOutput), nil
			}
			if a == "update" {
				return []byte("network error"), fmt.Errorf("exit status 1")
			}
		}
		return nil, nil
	}
	adapter := NewCopilotAdapter(runner)
	err := adapter.EnsureMarketplace("summon-marketplace", "ai-summon/summon-marketplace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update")
}

func TestClaudeAdapter_EnsureMarketplace_UpdateFailure(t *testing.T) {
	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	runner.RunFunc = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "list" {
				return []byte(`[{"name":"summon-marketplace","source":"ai-summon/summon-marketplace"}]`), nil
			}
			if a == "update" {
				return []byte("network error"), fmt.Errorf("exit status 1")
			}
		}
		return nil, nil
	}
	adapter := NewClaudeAdapter(runner)
	err := adapter.EnsureMarketplace("summon-marketplace", "ai-summon/summon-marketplace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update")
}

func TestCopilotAdapter_FindPluginDir_ConfigJSON(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	// Create plugin directory
	pluginDir := filepath.Join(home, ".copilot", "installed-plugins", "bmw-ai-marketplace", "sli-reprocessing")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	// Write config.json with camelCase key (as copilot CLI writes it)
	config := map[string]interface{}{
		"installedPlugins": []map[string]interface{}{
			{
				"name":       "sli-reprocessing",
				"marketplace": "bmw-ai-marketplace",
				"cache_path": pluginDir,
			},
		},
	}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	configDir := filepath.Join(home, ".copilot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644))

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapter := NewCopilotAdapter(runner)

	dir, err := adapter.FindPluginDir("sli-reprocessing", ScopeUser)
	require.NoError(t, err)
	assert.Equal(t, pluginDir, dir)
}

func TestCopilotAdapter_FindPluginDir_DynamicScan(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	// Create plugin directory under a non-standard marketplace (no config.json)
	pluginDir := filepath.Join(home, ".copilot", "installed-plugins", "custom-marketplace", "my-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapter := NewCopilotAdapter(runner)

	dir, err := adapter.FindPluginDir("my-plugin", ScopeUser)
	require.NoError(t, err)
	assert.Equal(t, pluginDir, dir)
}

func TestCopilotAdapter_FindPluginDir_NotFound(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	runner := NewFakeRunner()
	runner.LookPaths["copilot"] = "/usr/local/bin/copilot"
	adapter := NewCopilotAdapter(runner)

	_, err := adapter.FindPluginDir("nonexistent", ScopeUser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClaudeAdapter_FindPluginDir_DynamicScan(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	// Create plugin directory under a non-standard marketplace with version subdirectory
	versionDir := filepath.Join(home, ".claude", "plugins", "cache", "custom-marketplace", "my-plugin", "0.2.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))

	runner := NewFakeRunner()
	runner.LookPaths["claude"] = "/usr/local/bin/claude"
	adapter := NewClaudeAdapter(runner)

	dir, err := adapter.FindPluginDir("my-plugin", ScopeUser)
	require.NoError(t, err)
	assert.Equal(t, versionDir, dir)
}
