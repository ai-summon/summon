package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/user/summon/internal/git"
	"github.com/user/summon/internal/installer"
	"github.com/user/summon/internal/manifest"
	"github.com/user/summon/internal/marketplace"
	"github.com/user/summon/internal/platform"
	"github.com/user/summon/internal/registry"
	"github.com/user/summon/internal/resolver"
	"github.com/user/summon/internal/store"
	"github.com/user/summon/internal/ui"
)

var updateCmd = &cobra.Command{
	Use:   "update [package]",
	Short: "Update a package to the latest version",
	Long:  "Update a specific package or all packages. GitHub packages fetch latest; local packages regenerate marketplace views.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUpdate,
}

var updateGlobal bool
var updateScope string

func init() {
	updateCmd.Flags().BoolVarP(&updateGlobal, "global", "g", false, "Update in global scope")
	updateCmd.Flags().StringVar(&updateScope, "scope", "", "Update scope: user, project, or local")
	rootCmd.AddCommand(updateCmd)
}

// runUpdate refreshes one or all installed packages.
// GitHub-sourced packages are fetched/checked-out to the latest ref;
// local packages just regenerate their marketplace plugin.json.
func runUpdate(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		scope, err := resolveExistingPackageScope(projectDir, args[0], updateScope, updateGlobal)
		if err != nil {
			return err
		}
		return runScopedUpdate(projectDir, scope, args[0])
	}

	scope, err := resolveInstallScope(updateScope, updateGlobal)
	if err != nil {
		return err
	}
	return runScopedUpdate(projectDir, scope, "")
}

func runScopedUpdate(projectDir string, scope platform.Scope, packageName string) error {
	paths := installer.ResolvePaths(scope, projectDir)
	reg, err := registry.Load(paths.RegistryPath)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	s := store.New(paths.StoreDir)
	var toUpdate []string

	if packageName != "" {
		toUpdate = []string{packageName}
	} else {
		for name := range reg.Packages {
			toUpdate = append(toUpdate, name)
		}
	}

	if len(toUpdate) == 0 {
		ui.Info("No packages to update.")
		return nil
	}

	updated := 0
	for _, name := range toUpdate {
		entry, _ := reg.Get(name)
		storePath := s.PackagePath(name)

		switch entry.Source.Type {
		case "github":
			if err := updateGitHubPackage(name, entry, storePath, paths, reg); err != nil {
				ui.Error("updating %s: %v", name, err)
				continue
			}
			updated++
		case "local":
			m, mErr := manifest.Load(storePath)
			if mErr != nil {
				ui.Error("reading manifest for %s: %v", name, mErr)
				continue
			}
			if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
				ui.Error("regenerating plugin.json for %s: %v", name, err)
				continue
			}
			ui.Success("Regenerated marketplace views for %s", name)
			updated++
		}
	}

	// Regenerate platform-level marketplace views after all updates.
	for _, a := range platform.AllAdapters(projectDir) {
		platformDir := filepath.Join(paths.PlatformsDir, a.Name())
		_ = marketplace.Generate(a.Name(), paths.MarketplaceName, paths.StoreDir, platformDir, reg)
	}

	if err := reg.Save(paths.RegistryPath); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}

	ui.Detail("Updated %d package(s) in %s scope", updated, scope.String())
	return nil
}

// updateGitHubPackage fetches the latest tags, resolves the newest ref,
// checks it out (or pulls HEAD), and updates the registry entry if the
// commit SHA has changed.
func updateGitHubPackage(name string, entry registry.Entry, storePath string, paths installer.Paths, reg *registry.Registry) error {
	ui.Info("Checking %s for updates...", name)

	if err := git.FetchTags(storePath); err != nil {
		return err
	}

	latestRef, err := resolver.ResolveLatest(storePath)
	if err != nil {
		return err
	}

	if latestRef != "HEAD" {
		if err := git.Checkout(storePath, latestRef); err != nil {
			return err
		}
	} else {
		if err := git.Pull(storePath); err != nil {
			return err
		}
	}

	newSHA, err := git.RevParseHEAD(storePath)
	if err != nil {
		return err
	}

	if newSHA == entry.Source.SHA {
		ui.Info("%s is already up to date", name)
		return nil
	}

	m, err := manifest.Load(storePath)
	if err != nil {
		return err
	}

	if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
		return err
	}

	reg.Add(name, registry.Entry{
		Version: m.Version,
		Source: registry.Source{
			Type: "github",
			URL:  entry.Source.URL,
			Ref:  latestRef,
			SHA:  newSHA,
		},
		Platforms: entry.Platforms,
	})

	ui.Success("Updated %s to %s (%s)", name, m.Version, newSHA[:8])
	return nil
}
