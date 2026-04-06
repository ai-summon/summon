package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/summon/internal/installer"
	"github.com/user/summon/internal/manifest"
	"github.com/user/summon/internal/marketplace"
	"github.com/user/summon/internal/platform"
	"github.com/user/summon/internal/registry"
	"github.com/user/summon/internal/store"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <package>",
	Short: "Remove an installed package",
	Args:  cobra.ExactArgs(1),
	RunE:  runUninstall,
}

var uninstallGlobal bool
var uninstallScope string

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallGlobal, "global", "g", false, "Remove from global scope")
	uninstallCmd.Flags().StringVar(&uninstallScope, "scope", "", "Remove from scope: user, project, or local")
	rootCmd.AddCommand(uninstallCmd)
}

// runUninstall removes a package from the store, deletes its registry entry,
// regenerates marketplace views, and (if no packages remain) unregisters the
// marketplace from every active platform.
func runUninstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	scope, err := resolveExistingPackageScope(projectDir, name, uninstallScope, uninstallGlobal)
	if err != nil {
		return err
	}

	paths := installer.ResolvePaths(scope, projectDir)
	reg, err := registry.Load(paths.RegistryPath)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if !reg.Has(name) {
		return fmt.Errorf("package %q is not installed", name)
	}

	s := store.New(paths.StoreDir)

	// Before removing from the store, dematerialize any workspace component
	// links that were created for Copilot project/local scopes.
	if scope == platform.ScopeProject || scope == platform.ScopeLocal {
		storePath := s.PackagePath(name)
		if m, err := manifest.Load(storePath); err == nil {
			for _, a := range platform.DetectActive(projectDir) {
				if mat, ok := a.(platform.Materializer); ok {
					_ = mat.RemoveMaterialized(name, m, scope)
				}
			}
		}
	}

	if err := s.Remove(name); err != nil {
		return fmt.Errorf("removing from store: %w", err)
	}

	reg.Remove(name)
	if err := reg.Save(paths.RegistryPath); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}

	// Regenerate marketplace views so removed package disappears.
	for _, a := range platform.AllAdapters(projectDir) {
		platformDir := fmt.Sprintf("%s/%s", paths.PlatformsDir, a.Name())
		if err := marketplace.Generate(a.Name(), paths.MarketplaceName, paths.StoreDir, platformDir, reg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to regenerate marketplace for %s: %v\n", a.Name(), err)
		}
	}

	// Disable the plugin on all active platforms.
	for _, a := range platform.DetectActive(projectDir) {
		if err := a.DisablePlugin(name, paths.MarketplaceName, paths.StoreDir, paths.Scope); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to disable plugin on %s: %v\n", a.Name(), err)
		}
	}

	// If no packages remain, fully unregister from active platforms.
	if len(reg.Packages) == 0 {
		for _, a := range platform.DetectActive(projectDir) {
			if err := a.Unregister(paths.MarketplaceName, paths.Scope); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to unregister from %s: %v\n", a.Name(), err)
			}
		}
	}

	fmt.Fprintf(os.Stdout, "Uninstalled %s\n", name)
	return nil
}
