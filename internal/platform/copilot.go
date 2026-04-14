package platform

import (
	"fmt"
	"regexp"
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
	output, err := c.runner.Run("copilot", "plugin", "install", source)
	if err != nil {
		return cliError("copilot install", output, err)
	}
	return nil
}

func (c *CopilotAdapter) Uninstall(name string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	output, err := c.runner.Run("copilot", "plugin", "uninstall", name)
	if err != nil {
		return cliError("copilot uninstall", output, err)
	}
	return nil
}

func (c *CopilotAdapter) Update(name string, scope Scope) error {
	if err := ValidateScope(c, scope); err != nil {
		return err
	}
	output, err := c.runner.Run("copilot", "plugin", "update", name)
	if err != nil {
		return cliError("copilot update", output, err)
	}
	return nil
}

func (c *CopilotAdapter) ListInstalled(scope Scope) ([]InstalledPlugin, error) {
	if err := ValidateScope(c, scope); err != nil {
		return nil, err
	}
	// Copilot CLI does not support --json; parse human-readable text output
	output, err := c.runner.Run("copilot", "plugin", "list")
	if err != nil {
		return nil, fmt.Errorf("copilot list failed: %w", err)
	}
	return parseCopilotPluginList(output, c.Name())
}

// parseCopilotPluginList parses text output from `copilot plugin list`.
// Format: "  • plugin-name (v1.2.3)" or "  • plugin-name@marketplace"
var copilotPluginLine = regexp.MustCompile(`•\s+(\S+)`)

func parseCopilotPluginList(output []byte, plat string) ([]InstalledPlugin, error) {
	lines := strings.Split(string(output), "\n")
	var plugins []InstalledPlugin
	for _, line := range lines {
		matches := copilotPluginLine.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		raw := matches[1]
		// Strip @marketplace suffix and version parenthetical for the name
		name := raw
		if idx := strings.Index(name, "@"); idx > 0 {
			name = name[:idx]
		}
		plugins = append(plugins, InstalledPlugin{
			Name:     name,
			Source:   raw,
			Platform: plat,
			Scope:    string(ScopeUser),
		})
	}
	return plugins, nil
}
