package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ClaudeAdapter implements the Adapter interface for Claude Code CLI.
type ClaudeAdapter struct {
	runner CommandRunner
	cwd    string
}

// NewClaudeAdapter creates a new ClaudeAdapter.
func NewClaudeAdapter(runner CommandRunner) *ClaudeAdapter {
	cwd, _ := os.Getwd()
	return &ClaudeAdapter{runner: runner, cwd: cwd}
}

// NewClaudeAdapterWithCwd creates a ClaudeAdapter with an explicit working directory (for testing).
func NewClaudeAdapterWithCwd(runner CommandRunner, cwd string) *ClaudeAdapter {
	return &ClaudeAdapter{runner: runner, cwd: cwd}
}

func (c *ClaudeAdapter) Name() string { return "claude" }

func (c *ClaudeAdapter) Detect() bool {
	_, err := c.runner.LookPath("claude")
	return err == nil
}

func (c *ClaudeAdapter) SupportedScopes() []Scope {
	return []Scope{ScopeUser, ScopeProject, ScopeLocal}
}

func (c *ClaudeAdapter) Install(source string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	args := []string{"plugin", "install", source}
	if scope != ScopeUser {
		args = append(args, "--scope", string(scope))
	}
	output, err := c.runner.Run("claude", args...)
	if err != nil {
		return cliError("claude install", output, err)
	}
	return nil
}

func (c *ClaudeAdapter) Uninstall(name string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	args := []string{"plugin", "uninstall", name}
	if scope != ScopeUser {
		args = append(args, "--scope", string(scope))
	}
	output, err := c.runner.Run("claude", args...)
	if err != nil {
		return cliError("claude uninstall", output, err)
	}
	return nil
}

func (c *ClaudeAdapter) Update(name string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	args := []string{"plugin", "update", name}
	if scope != ScopeUser {
		args = append(args, "--scope", string(scope))
	}
	output, err := c.runner.Run("claude", args...)
	if err != nil {
		return cliError("claude update", output, err)
	}
	return nil
}

func (c *ClaudeAdapter) ListInstalled(scope Scope) ([]InstalledPlugin, error) {
	if err := ValidateScope(c, scope); err != nil {
		return nil, err
	}
	// claude plugin list --json always returns all scopes; filtering is done in code
	args := []string{"plugin", "list", "--json"}
	output, err := c.runner.Run("claude", args...)
	if err != nil {
		return nil, fmt.Errorf("claude list failed: %w", err)
	}
	plugins, err := parseClaudePluginList(output, c.Name())
	if err != nil {
		return nil, err
	}
	// Filter out project/local-scope plugins from other projects
	var filtered []InstalledPlugin
	for _, p := range plugins {
		if (p.Scope == string(ScopeProject) || p.Scope == string(ScopeLocal)) && p.ProjectPath != "" {
			if !isUnderPath(c.cwd, p.ProjectPath) {
				continue
			}
		}
		filtered = append(filtered, p)
	}
	return filtered, nil
}

// parseClaudePluginList parses JSON output from `claude plugin list --json`.
// Actual format: [{"id":"name@marketplace","version":"...","scope":"...","enabled":true,...}]
func parseClaudePluginList(output []byte, plat string) ([]InstalledPlugin, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var raw []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Source      string `json:"source"`
		Version     string `json:"version"`
		Scope       string `json:"scope"`
		Enabled     bool   `json:"enabled"`
		ProjectPath string `json:"projectPath"`
	}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse claude plugin list JSON: %w", err)
	}

	var plugins []InstalledPlugin
	for _, r := range raw {
		name := r.Name
		source := r.Source
		// Claude uses "id" field with format "name@marketplace"
		if name == "" && r.ID != "" {
			name = r.ID
			if idx := strings.Index(name, "@"); idx > 0 {
				name = name[:idx]
			}
		}
		if source == "" {
			source = r.ID
		}
		plugins = append(plugins, InstalledPlugin{
			Name:        name,
			Source:      source,
			Platform:    plat,
			Scope:       r.Scope,
			ProjectPath: r.ProjectPath,
		})
	}
	return plugins, nil
}
