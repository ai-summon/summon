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
	_, err := c.runner.Run("claude", args...)
	if err != nil {
		return fmt.Errorf("claude install failed: %w", err)
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
	_, err := c.runner.Run("claude", args...)
	if err != nil {
		return fmt.Errorf("claude uninstall failed: %w", err)
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
	_, err := c.runner.Run("claude", args...)
	if err != nil {
		return fmt.Errorf("claude update failed: %w", err)
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

func parseClaudePluginList(output []byte, platform string) ([]InstalledPlugin, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var raw []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse claude plugin list JSON: %w", err)
	}

	plugins := make([]InstalledPlugin, len(raw))
	for i, r := range raw {
		plugins[i] = InstalledPlugin{
			Name:     r.Name,
			Source:   r.Source,
			Platform: platform,
		}
	}
	return plugins, nil
}
