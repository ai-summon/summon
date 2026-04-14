package cli

import (
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/store"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <package>",
	Short: "Remove an installed package",
	Args:  cobra.ExactArgs(1),
	RunE:  runUninstall,
}

var (
	uninstallGlobal  bool
	uninstallProject bool
	uninstallScope   string
)

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallGlobal, "global", "g", false, "Shortcut for --scope user")
	uninstallCmd.Flags().BoolVarP(&uninstallProject, "project", "p", false, "Shortcut for --scope project")
	uninstallCmd.Flags().StringVar(&uninstallScope, "scope", "", "Target scope. One of local, project, user")
	uninstallCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
	rootCmd.AddCommand(uninstallCmd)
}

// runUninstall removes a package from the store, deletes its registry entry,
// and delegates removal to each platform via adapter.RemovePackage.
func runUninstall(cmd *cobra.Command, args []string) error {
	name := args[0]
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	scope, err := resolveExistingPackageScope(projectDir, name, uninstallScope, uninstallGlobal, uninstallProject)
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

	if err := s.Remove(name); err != nil {
		return fmt.Errorf("removing from store: %w", err)
	}

	reg.Remove(name)
	if err := reg.Save(paths.RegistryPath); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}

	// Delegate removal to each active platform via CLI
	for _, a := range platform.DetectActive(projectDir) {
		if platform.SupportsScope(a, scope) {
			if err := a.RemovePackage(name, scope); err != nil {
				installer.StatusErr("Warning", "failed to remove from %s: %v", a.Name(), err)
			}
		}
	}

	installer.Status("Uninstalled", "%s from %s scope", name, scope.String())
	return nil
}
