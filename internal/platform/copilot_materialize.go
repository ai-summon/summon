package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ai-summon/summon/internal/fsutil"
)

// ComponentsInfo describes the component directories provided by a package.
// It is satisfied by *manifest.Manifest and by the testManifest stub used
// in unit tests.
type ComponentsInfo interface {
	GetName() string
	GetSkills() string
	GetAgents() string
	GetHooks() string
	GetMCP() string
}

// MaterializeComponents copies or links package component directories into the
// documented workspace discovery paths for the requested Copilot scope.
//
// Project scope target paths (documented by VS Code and Copilot CLI):
//   - Skills  → .github/skills/<name>
//   - Agents  → .github/agents/<name>
//   - MCP     → .vscode/mcp.json (merge – TODO)
//   - Hooks   → not supported for Copilot project scope; returns error
//
// Local scope target paths:
//   - Skills  → .claude/skills/<name>
//   - Agents  → .claude/agents/<name>
//   - MCP     → no documented personal-in-workspace Copilot path; returns error
//   - Hooks   → .claude/settings.local.json (not yet implemented; returns error)
func (v *CopilotAdapter) MaterializeComponents(pkgDir string, m ComponentsInfo, scope Scope) error {
	name := m.GetName()
	var unsupported []string

	switch scope {
	case ScopeProject:
		if s := m.GetSkills(); s != "" {
			if err := v.linkComponent(filepath.Join(pkgDir, s),
				filepath.Join(v.ProjectDir, ".github", "skills", name)); err != nil {
				return fmt.Errorf("materializing skills for %s (project): %w", name, err)
			}
		}
		if a := m.GetAgents(); a != "" {
			if err := v.linkComponent(filepath.Join(pkgDir, a),
				filepath.Join(v.ProjectDir, ".github", "agents", name)); err != nil {
				return fmt.Errorf("materializing agents for %s (project): %w", name, err)
			}
		}
		if m.GetHooks() != "" {
			unsupported = append(unsupported, "hooks")
		}
		if m.GetMCP() != "" {
			// MCP project scope is supported via .vscode/mcp.json but not yet
			// implemented; for now it is unsupported.
			unsupported = append(unsupported, "mcp")
		}
		if len(unsupported) > 0 {
			return fmt.Errorf("Copilot project scope does not support these components for %s: %v; "+
				"use --scope user to install with full Copilot plugin support", name, unsupported)
		}
	case ScopeLocal:
		if s := m.GetSkills(); s != "" {
			if err := v.linkComponent(filepath.Join(pkgDir, s),
				filepath.Join(v.ProjectDir, ".claude", "skills", name)); err != nil {
				return fmt.Errorf("materializing skills for %s (local): %w", name, err)
			}
		}
		if a := m.GetAgents(); a != "" {
			if err := v.linkComponent(filepath.Join(pkgDir, a),
				filepath.Join(v.ProjectDir, ".claude", "agents", name)); err != nil {
				return fmt.Errorf("materializing agents for %s (local): %w", name, err)
			}
		}
		if m.GetHooks() != "" {
			unsupported = append(unsupported, "hooks")
		}
		if m.GetMCP() != "" {
			unsupported = append(unsupported, "mcp (no documented personal-in-workspace Copilot path for local scope)")
		}
		if len(unsupported) > 0 {
			return fmt.Errorf("Copilot local scope does not support these components for %s: %v; "+
				"use --scope project or --scope user to install with those components", name, unsupported)
		}
	}

	return nil
}

// RemoveMaterialized removes any workspace links created by MaterializeComponents
// for the given package name and scope.
func (v *CopilotAdapter) RemoveMaterialized(pkgName string, m ComponentsInfo, scope Scope) error {
	switch scope {
	case ScopeProject:
		if m.GetSkills() != "" {
			_ = os.RemoveAll(filepath.Join(v.ProjectDir, ".github", "skills", pkgName))
		}
		if m.GetAgents() != "" {
			_ = os.RemoveAll(filepath.Join(v.ProjectDir, ".github", "agents", pkgName))
		}
	case ScopeLocal:
		if m.GetSkills() != "" {
			_ = os.RemoveAll(filepath.Join(v.ProjectDir, ".claude", "skills", pkgName))
		}
		if m.GetAgents() != "" {
			_ = os.RemoveAll(filepath.Join(v.ProjectDir, ".claude", "agents", pkgName))
		}
	}
	return nil
}

// linkComponent creates a directory link from target → source, creating the
// parent directory if needed and replacing any existing link.
func (v *CopilotAdapter) linkComponent(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating parent: %w", err)
	}
	// Remove existing link/dir.
	if _, err := os.Lstat(target); err == nil {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("removing existing target: %w", err)
		}
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	return fsutil.CreateDirLink(abs, target)
}
