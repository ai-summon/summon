package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ai-summon/summon/internal/depcheck"
	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/marketplace"
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
	uninstallForce   bool
)

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallGlobal, "global", "g", false, "Shortcut for --scope user")
	uninstallCmd.Flags().BoolVarP(&uninstallProject, "project", "p", false, "Shortcut for --scope project")
	uninstallCmd.Flags().StringVar(&uninstallScope, "scope", "", "Target scope. One of local, project, user")
	uninstallCmd.Flags().BoolVar(&uninstallForce, "force", false, "Skip reverse dependency check")
	uninstallCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
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

	// Reverse dependency check (skip if --force)
	if !uninstallForce {
		allScopes := platform.ScopePrecedence()
		registries := make(map[platform.Scope]*registry.Registry)
		for _, s := range allScopes {
			p := installer.ResolvePaths(s, projectDir)
			r, loadErr := registry.Load(p.RegistryPath)
			if loadErr != nil {
				continue
			}
			registries[s] = r
		}

		reverseDeps := depcheck.FindReverseDeps(
			name,
			allScopes,
			registries,
			func(s platform.Scope, n string) string {
				p := installer.ResolvePaths(s, projectDir)
				return p.StoreDir + "/" + n
			},
			func(pkgDir string) (*manifest.Manifest, error) {
				return manifest.Load(pkgDir)
			},
		)

		if len(reverseDeps) > 0 {
			installer.StatusErr("Warning", "the following packages depend on %s:", name)
			for _, rd := range reverseDeps {
				constraint := rd.Constraint
				if constraint == "" {
					constraint = "(any version)"
				}
				fmt.Fprintf(installer.Stderr, "%*s - %s (%s) requires %s %s\n",
					12, "", rd.PackageName, rd.Scope, name, constraint)
			}

			if !installer.IsInteractive() {
				return fmt.Errorf("package %q has dependents; use --force to remove anyway", name)
			}

			fmt.Fprintf(installer.Stderr, "\n%*s Proceed with uninstall? (y/N) ", 12, "")
			scanner := bufio.NewScanner(installer.Stdin)
			if !scanner.Scan() {
				installer.Status("Cancelled", "uninstall of %s", name)
				return nil
			}
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "y" && answer != "yes" {
				installer.Status("Cancelled", "uninstall of %s", name)
				return nil
			}
		}
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
			installer.StatusErr("Warning", "failed to regenerate marketplace for %s: %v", a.Name(), err)
		}
	}

	// Disable the plugin on all active platforms.
	for _, a := range platform.DetectActive(projectDir) {
		if err := a.DisablePlugin(name, paths.MarketplaceName, paths.StoreDir, paths.Scope); err != nil {
			installer.StatusErr("Warning", "failed to disable plugin on %s: %v", a.Name(), err)
		}
	}

	// If no packages remain, fully unregister from active platforms.
	if len(reg.Packages) == 0 {
		for _, a := range platform.DetectActive(projectDir) {
			if err := a.Unregister(paths.MarketplaceName, paths.Scope); err != nil {
				installer.StatusErr("Warning", "failed to unregister from %s: %v", a.Name(), err)
			}
		}
	}

	installer.Status("Uninstalled", "%s from %s scope", name, scope.String())

	// Remind about packages with now-unmet deps
	if !uninstallForce {
		allScopes := platform.ScopePrecedence()
		registries := make(map[platform.Scope]*registry.Registry)
		for _, s := range allScopes {
			p := installer.ResolvePaths(s, projectDir)
			r, loadErr := registry.Load(p.RegistryPath)
			if loadErr != nil {
				continue
			}
			registries[s] = r
		}
		reverseDeps := depcheck.FindReverseDeps(
			name,
			allScopes,
			registries,
			func(s platform.Scope, n string) string {
				p := installer.ResolvePaths(s, projectDir)
				return p.StoreDir + "/" + n
			},
			func(pkgDir string) (*manifest.Manifest, error) {
				return manifest.Load(pkgDir)
			},
		)
		if len(reverseDeps) > 0 {
			installer.StatusErr("Note", "the following packages now have unmet dependencies:")
			for _, rd := range reverseDeps {
				fmt.Fprintf(installer.Stderr, "%*s - %s (%s)\n", 12, "", rd.PackageName, rd.Scope)
			}
		}
	}
	return nil
}
