package platform

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ClaudeAdapter implements the Adapter interface for Claude Code CLI.
type ClaudeAdapter struct {
	runner CommandRunner
}

// NewClaudeAdapter creates a new ClaudeAdapter.
func NewClaudeAdapter(runner CommandRunner) *ClaudeAdapter {
	return &ClaudeAdapter{runner: runner}
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
	args := []string{"plugin", "list", "--json"}
	if scope != ScopeUser {
		args = append(args, "--scope", string(scope))
	}
	output, err := c.runner.Run("claude", args...)
	if err != nil {
		return nil, fmt.Errorf("claude list failed: %w", err)
	}
	return parseClaudePluginList(output, c.Name())
}

// parseClaudePluginList parses JSON output from `claude plugin list --json`.
// Actual format: [{"id":"name@marketplace","version":"...","scope":"...","enabled":true,...}]
func parseClaudePluginList(output []byte, plat string) ([]InstalledPlugin, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var raw []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Source  string `json:"source"`
		Version string `json:"version"`
		Scope   string `json:"scope"`
		Enabled bool   `json:"enabled"`
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
			Name:     name,
			Source:   source,
			Platform: plat,
		})
	}
	return plugins, nil
}
