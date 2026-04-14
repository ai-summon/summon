package platform

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CopilotAdapter implements the Adapter interface for GitHub Copilot CLI.
type CopilotAdapter struct {
	runner CommandRunner
}

// NewCopilotAdapter creates a new CopilotAdapter.
func NewCopilotAdapter(runner CommandRunner) *CopilotAdapter {
	return &CopilotAdapter{runner: runner}
}

func (c *CopilotAdapter) Name() string { return "copilot" }

func (c *CopilotAdapter) Detect() bool {
	_, err := c.runner.LookPath("copilot")
	return err == nil
}

func (c *CopilotAdapter) SupportedScopes() []Scope {
	return []Scope{ScopeUser}
}

func (c *CopilotAdapter) Install(source string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	_, err := c.runner.Run("copilot", "plugin", "install", source)
	if err != nil {
		return fmt.Errorf("copilot install failed: %w", err)
	}
	return nil
}

func (c *CopilotAdapter) Uninstall(name string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	_, err := c.runner.Run("copilot", "plugin", "uninstall", name)
	if err != nil {
		return fmt.Errorf("copilot uninstall failed: %w", err)
	}
	return nil
}

func (c *CopilotAdapter) Update(name string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	_, err := c.runner.Run("copilot", "plugin", "update", name)
	if err != nil {
		return fmt.Errorf("copilot update failed: %w", err)
	}
	return nil
}

func (c *CopilotAdapter) ListInstalled(scope Scope) ([]InstalledPlugin, error) {
	if err := ValidateScope(c, scope); err != nil {
		return nil, err
	}
	output, err := c.runner.Run("copilot", "plugin", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("copilot list failed: %w", err)
	}
	return parseCopilotPluginList(output, c.Name())
}

func parseCopilotPluginList(output []byte, platform string) ([]InstalledPlugin, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var raw []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse copilot plugin list JSON: %w", err)
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
