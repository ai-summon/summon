package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeAdapter implements the Adapter interface for Claude Code.
// It delegates all plugin management to the `claude` CLI — summon never reads
// or writes any Claude settings file.
type ClaudeAdapter struct {
	ProjectDir string
	Runner     CmdRunner
}

func (c *ClaudeAdapter) Name() string {
	return "claude"
}

func (c *ClaudeAdapter) Detect() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".claude"))
	return err == nil
}

func (c *ClaudeAdapter) SupportedScopes() []Scope {
	return []Scope{ScopeLocal, ScopeProject, ScopeUser}
}

// DiscoverPackage registers a marketplace and installs a plugin via the Claude CLI.
// Two-step flow: marketplace add → plugin install.
func (c *ClaudeAdapter) DiscoverPackage(pkgPath string, pkgName string, scope Scope) error {
	absPath, err := filepath.Abs(pkgPath)
	if err != nil {
		absPath = pkgPath
	}

	// Step 1: Register the package path as a marketplace (idempotent).
	scopeStr := scope.String()
	_, _, err = c.Runner.Run("claude", "plugin", "marketplace", "add", absPath, "--scope", scopeStr)
	if err != nil {
		return fmt.Errorf("claude marketplace add: %w", err)
	}

	// Step 2: Install the plugin from the marketplace.
	// The marketplace name is derived from the directory name.
	marketplaceName := filepath.Base(absPath)
	pluginRef := pkgName + "@" + marketplaceName
	_, _, err = c.Runner.Run("claude", "plugin", "install", pluginRef, "--scope", scopeStr)
	if err != nil {
		return fmt.Errorf("claude plugin install: %w", err)
	}

	return nil
}

// RemovePackage uninstalls a plugin and removes its marketplace via the Claude CLI.
func (c *ClaudeAdapter) RemovePackage(pkgName string, scope Scope) error {
	// Best-effort: uninstall the plugin.
	_, _, err := c.Runner.Run("claude", "plugin", "uninstall", pkgName)
	if err != nil {
		// Log but don't fail — plugin may already be removed.
		fmt.Fprintf(os.Stderr, "Warning: claude plugin uninstall %s: %v\n", pkgName, err)
	}
	return nil
}

// CleanOrphans is a best-effort cleanup for when Claude is no longer detected.
func (c *ClaudeAdapter) CleanOrphans() error {
	// Nothing to do — Claude CLI manages its own state.
	return nil
}
