package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// Each subdirectory inside a component path (e.g., skills/brainstorm/) is
// linked individually so that SKILL.md / .agent.md files sit exactly one
// directory level deep from the discovery root.
//
// Project scope target paths (documented by VS Code and Copilot CLI):
//   - Skills  → .github/skills/<skill-subdir>
//   - Agents  → .github/agents/<agent-subdir>
//   - MCP     → .vscode/mcp.json (merge – TODO)
//   - Hooks   → not supported for Copilot project scope; returns error
//
// Local scope target paths:
//   - Skills  → .claude/skills/<skill-subdir>
//   - Agents  → .claude/agents/<agent-subdir>
//   - MCP     → no documented personal-in-workspace Copilot path; returns error
//   - Hooks   → .claude/settings.local.json (not yet implemented; returns error)
func (v *CopilotAdapter) MaterializeComponents(pkgDir string, m ComponentsInfo, scope Scope) error {
	name := m.GetName()
	var unsupported []string

	switch scope {
	case ScopeProject:
		if s := m.GetSkills(); s != "" {
			if err := v.linkComponentSubdirs(pkgDir, filepath.Join(pkgDir, s),
				filepath.Join(v.ProjectDir, ".github", "skills"), name, "skills"); err != nil {
				return err
			}
		}
		if a := m.GetAgents(); a != "" {
			if err := v.linkComponentSubdirs(pkgDir, filepath.Join(pkgDir, a),
				filepath.Join(v.ProjectDir, ".github", "agents"), name, "agents"); err != nil {
				return err
			}
		}
		if m.GetHooks() != "" {
			unsupported = append(unsupported, "hooks")
		}
		if m.GetMCP() != "" {
			unsupported = append(unsupported, "mcp")
		}
		if len(unsupported) > 0 {
			return fmt.Errorf("Copilot project scope does not support these components for %s: %v; "+
				"use --scope user to install with full Copilot plugin support", name, unsupported)
		}
	case ScopeLocal:
		if s := m.GetSkills(); s != "" {
			if err := v.linkComponentSubdirs(pkgDir, filepath.Join(pkgDir, s),
				filepath.Join(v.ProjectDir, ".claude", "skills"), name, "skills"); err != nil {
				return err
			}
		}
		if a := m.GetAgents(); a != "" {
			if err := v.linkComponentSubdirs(pkgDir, filepath.Join(pkgDir, a),
				filepath.Join(v.ProjectDir, ".claude", "agents"), name, "agents"); err != nil {
				return err
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
// for the given package name and scope. It enumerates subdirectories of each
// component path to determine which individual links to remove.
func (v *CopilotAdapter) RemoveMaterialized(pkgName string, pkgDir string, m ComponentsInfo, scope Scope) error {
	switch scope {
	case ScopeProject:
		if s := m.GetSkills(); s != "" {
			v.removeComponentSubdirs(filepath.Join(pkgDir, s),
				filepath.Join(v.ProjectDir, ".github", "skills"))
		}
		if a := m.GetAgents(); a != "" {
			v.removeComponentSubdirs(filepath.Join(pkgDir, a),
				filepath.Join(v.ProjectDir, ".github", "agents"))
		}
	case ScopeLocal:
		if s := m.GetSkills(); s != "" {
			v.removeComponentSubdirs(filepath.Join(pkgDir, s),
				filepath.Join(v.ProjectDir, ".claude", "skills"))
		}
		if a := m.GetAgents(); a != "" {
			v.removeComponentSubdirs(filepath.Join(pkgDir, a),
				filepath.Join(v.ProjectDir, ".claude", "agents"))
		}
	}
	return nil
}

// linkComponentSubdirs enumerates immediate subdirectories of componentDir and
// creates an individual link in discoveryRoot for each. If the componentDir
// does not exist, an error is returned. If it has no subdirectories, no links
// are created and nil is returned.
func (v *CopilotAdapter) linkComponentSubdirs(pkgDir, componentDir, discoveryRoot, pkgName, componentType string) error {
	entries, err := os.ReadDir(componentDir)
	if err != nil {
		return fmt.Errorf("materializing %s for %s: reading component directory: %w", componentType, pkgName, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		source := filepath.Join(componentDir, entry.Name())
		target := filepath.Join(discoveryRoot, entry.Name())

		// Collision detection: if target already exists as a link pointing
		// to a different package, warn the user.
		if existing, err := os.Readlink(target); err == nil {
			absExisting, _ := filepath.Abs(existing)
			absPkgDir, _ := filepath.Abs(pkgDir)
			if !strings.HasPrefix(absExisting, absPkgDir+string(filepath.Separator)) && absExisting != absPkgDir {
				fmt.Fprintf(os.Stderr, "Warning: %s %q overwrites existing link → %s\n", componentType, entry.Name(), absExisting)
			}
		}

		if err := v.linkComponent(source, target); err != nil {
			return fmt.Errorf("materializing %s for %s: %w", componentType, pkgName, err)
		}
	}
	return nil
}

// removeComponentSubdirs enumerates immediate subdirectories of componentDir
// and removes the corresponding link from discoveryRoot for each, but only if
// the link still points into componentDir (collision-safe).
func (v *CopilotAdapter) removeComponentSubdirs(componentDir, discoveryRoot string) {
	entries, err := os.ReadDir(componentDir)
	if err != nil {
		return
	}
	absComponentDir, _ := filepath.Abs(componentDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		link := filepath.Join(discoveryRoot, entry.Name())
		// Only remove if the link still points into this package's component
		// directory. If another package overwrote the link (collision), leave
		// the new owner's link intact.
		if target, err := os.Readlink(link); err == nil {
			absTarget, _ := filepath.Abs(target)
			if !strings.HasPrefix(absTarget, absComponentDir+string(filepath.Separator)) {
				continue
			}
		}
		_ = os.RemoveAll(link)
	}
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
