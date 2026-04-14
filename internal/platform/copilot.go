package platform

import (
	"fmt"
	"os"
	"os/exec"
)

// CopilotAdapter implements the Adapter interface for GitHub Copilot.
// It delegates all plugin management to the `copilot` CLI — summon never reads
// or writes any VS Code settings file. Copilot only supports user scope.
type CopilotAdapter struct {
	ProjectDir string
	Runner     CmdRunner
}

func (v *CopilotAdapter) Name() string {
	return "copilot"
}

func (v *CopilotAdapter) Detect() bool {
	_, err := exec.LookPath("copilot")
	return err == nil
}

func (v *CopilotAdapter) SupportedScopes() []Scope {
	return []Scope{ScopeUser}
}

// DiscoverPackage installs a plugin via the Copilot CLI (direct install).
func (v *CopilotAdapter) DiscoverPackage(pkgPath string, pkgName string, scope Scope) error {
	_, _, err := v.Runner.Run("copilot", "plugin", "install", pkgPath)
	if err != nil {
		return fmt.Errorf("copilot plugin install: %w", err)
	}
	return nil
}

// RemovePackage uninstalls a plugin via the Copilot CLI.
func (v *CopilotAdapter) RemovePackage(pkgName string, scope Scope) error {
	_, _, err := v.Runner.Run("copilot", "plugin", "uninstall", pkgName)
	if err != nil {
		// Best-effort: log but don't fail.
		fmt.Fprintf(os.Stderr, "Warning: copilot plugin uninstall %s: %v\n", pkgName, err)
	}
	return nil
}

// CleanOrphans is a best-effort cleanup for when Copilot is no longer detected.
func (v *CopilotAdapter) CleanOrphans() error {
	// Nothing to do — Copilot CLI manages its own state.
	return nil
}
