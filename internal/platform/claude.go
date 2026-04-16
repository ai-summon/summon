package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			Version:     r.Version,
			Source:      source,
			Platform:    plat,
			Scope:       r.Scope,
			ProjectPath: r.ProjectPath,
		})
	}
	return plugins, nil
}

func (c *ClaudeAdapter) EnsureMarketplace(name, source string) error {
	// Check if marketplace is already registered
	marketplaces, err := c.ListMarketplaces()
	if err != nil {
		return fmt.Errorf("failed to list marketplaces: %w", err)
	}
	for _, m := range marketplaces {
		if m.Name == name {
			// Refresh the marketplace index to pick up newly added plugins
			output, err := c.runner.Run("claude", "plugin", "marketplace", "update", name)
			if err != nil {
				return fmt.Errorf("marketplace update failed: %w", cliError("claude marketplace update", output, err))
			}
			return nil
		}
	}

	// Add the marketplace
	output, err := c.runner.Run("claude", "plugin", "marketplace", "add", source)
	if err != nil {
		return cliError("claude marketplace add", output, err)
	}
	return nil
}

func (c *ClaudeAdapter) RemoveMarketplace(name string) error {
	output, err := c.runner.Run("claude", "plugin", "marketplace", "remove", name)
	if err != nil {
		return cliError("claude marketplace remove", output, err)
	}
	return nil
}

func (c *ClaudeAdapter) ListMarketplaces() ([]MarketplaceInfo, error) {
	output, err := c.runner.Run("claude", "plugin", "marketplace", "list", "--json")
	if err != nil {
		return nil, cliError("claude marketplace list", output, err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var raw []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Repo   string `json:"repo"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse claude marketplace list JSON: %w", err)
	}

	var marketplaces []MarketplaceInfo
	for _, r := range raw {
		source := r.Repo
		if source == "" {
			source = r.URL
		}
		if source == "" {
			source = r.Source
		}
		marketplaces = append(marketplaces, MarketplaceInfo{
			Name:   r.Name,
			Source: source,
		})
	}
	return marketplaces, nil
}

func (c *ClaudeAdapter) FindPluginDir(name string, scope Scope) (string, error) {
	if err := ValidateScope(c, scope); err != nil {
		return "", err
	}

	// Determine base path based on scope
	var basePath string
	switch scope {
	case ScopeUser:
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		basePath = filepath.Join(homeDir, ".claude", "plugins", "cache")
	case ScopeProject, ScopeLocal:
		basePath = filepath.Join(c.cwd, ".claude", "plugins", "cache")
	}

	// Strategy 1: Scan cache directory for marketplace/name/version pattern
	marketplaces := []string{"summon-marketplace", "claude-plugins-official"}
	for _, mkt := range marketplaces {
		mktDir := filepath.Join(basePath, mkt, name)
		if entries, err := os.ReadDir(mktDir); err == nil {
			// Find the latest version directory
			var latestVersion string
			for _, e := range entries {
				if e.IsDir() {
					latestVersion = e.Name()
				}
			}
			if latestVersion != "" {
				dir := filepath.Join(mktDir, latestVersion)
				return dir, nil
			}
		}
	}

	// Strategy 2: Read installed_plugins.json for explicit path info
	if scope == ScopeUser {
		homeDir, _ := os.UserHomeDir()
		metaPath := filepath.Join(homeDir, ".claude", "plugins", "installed_plugins.json")
		if data, err := os.ReadFile(metaPath); err == nil {
			var plugins []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			}
			if err := json.Unmarshal(data, &plugins); err == nil {
				for _, p := range plugins {
					pName := p.Name
					if idx := strings.Index(pName, "@"); idx > 0 {
						pName = pName[:idx]
					}
					if pName == name && p.Path != "" {
						if _, err := os.Stat(p.Path); err == nil {
							return p.Path, nil
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("plugin directory for %q not found; checked %s", name, basePath)
}