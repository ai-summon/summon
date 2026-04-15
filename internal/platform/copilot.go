package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Strip @marketplace suffix — copilot CLI uses bare plugin names
	if idx := strings.Index(name, "@"); idx > 0 {
		name = name[:idx]
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

func (c *CopilotAdapter) EnsureMarketplace(name, source string) error {
	// Check if marketplace is already registered
	marketplaces, err := c.ListMarketplaces()
	if err != nil {
		return fmt.Errorf("failed to list marketplaces: %w", err)
	}
	for _, m := range marketplaces {
		if m.Name == name {
			// Refresh the marketplace index to pick up newly added plugins
			output, err := c.runner.Run("copilot", "plugin", "marketplace", "update", name)
			if err != nil {
				return fmt.Errorf("marketplace update failed: %w", cliError("copilot marketplace update", output, err))
			}
			return nil
		}
	}

	// Add the marketplace
	output, err := c.runner.Run("copilot", "plugin", "marketplace", "add", source)
	if err != nil {
		return cliError("copilot marketplace add", output, err)
	}
	return nil
}

// copilotMarketplaceLine parses marketplace entries from copilot plugin marketplace list output.
// Built-in marketplaces use "◆" and user-registered use "•": both followed by "name (GitHub: owner/repo)".
var copilotMarketplaceLine = regexp.MustCompile(`[◆•]\s+(\S+)\s+\(GitHub:\s+(\S+)\)`)

func (c *CopilotAdapter) ListMarketplaces() ([]MarketplaceInfo, error) {
	output, err := c.runner.Run("copilot", "plugin", "marketplace", "list")
	if err != nil {
		return nil, cliError("copilot marketplace list", output, err)
	}

	lines := strings.Split(string(output), "\n")
	var marketplaces []MarketplaceInfo
	for _, line := range lines {
		matches := copilotMarketplaceLine.FindStringSubmatch(line)
		if len(matches) >= 3 {
			marketplaces = append(marketplaces, MarketplaceInfo{
				Name:   matches[1],
				Source: matches[2],
			})
		}
	}
	return marketplaces, nil
}

func (c *CopilotAdapter) FindPluginDir(name string, scope Scope) (string, error) {
	if err := ValidateScope(c, scope); err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Strategy 1: Read ~/.copilot/config.json for cache_path
	configPath := filepath.Join(homeDir, ".copilot", "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var config struct {
			InstalledPlugins []struct {
				Name      string `json:"name"`
				CachePath string `json:"cache_path"`
			} `json:"installed_plugins"`
		}
		if err := json.Unmarshal(data, &config); err == nil {
			for _, p := range config.InstalledPlugins {
				pName := p.Name
				if idx := strings.Index(pName, "@"); idx > 0 {
					pName = pName[:idx]
				}
				if pName == name && p.CachePath != "" {
					if _, err := os.Stat(p.CachePath); err == nil {
						return p.CachePath, nil
					}
				}
			}
		}
	}

	// Strategy 2: Check well-known marketplace directory paths
	basePath := filepath.Join(homeDir, ".copilot", "installed-plugins")
	marketplaces := []string{"summon-marketplace", "copilot-plugins", "awesome-copilot", "_direct"}
	for _, mkt := range marketplaces {
		dir := filepath.Join(basePath, mkt, name)
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	return "", fmt.Errorf("plugin directory for %q not found; checked %s", name, basePath)
}
