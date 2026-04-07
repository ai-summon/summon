package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ai-summon/summon/internal/git"
	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/marketplace"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/resolver"
	"github.com/ai-summon/summon/internal/store"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [package]",
	Short: "Update installed packages",
	Long:  "Update a specific package or all packages. Unpinned GitHub packages resolve to the latest tag; pinned packages refresh at their stored ref. Local packages regenerate marketplace views.",
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
// Unpinned GitHub packages resolve to the latest semver tag;
// pinned packages fetch and check out their stored ref.
// Local packages just regenerate their marketplace plugin.json.
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
		fmt.Fprintln(installer.Stdout, "No packages to update.")
		return nil
	}

	updated := 0
	for _, name := range toUpdate {
		entry, _ := reg.Get(name)
		storePath := s.PackagePath(name)

		switch entry.Source.Type {
		case "github":
			if err := updateGitHubPackage(name, entry, storePath, paths, reg); err != nil {
				installer.StatusErr("Error", "updating %s: %v", name, err)
				continue
			}
			updated++
		case "local":
			manifests, _, mErr := manifest.LoadOrInfer(storePath)
			if mErr != nil {
				installer.StatusErr("Error", "reading manifest for %s: %v", name, mErr)
				continue
			}
			m := manifests[0]
			if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
				installer.StatusErr("Error", "regenerating plugin.json for %s: %v", name, err)
				continue
			}
			installer.Status("Regenerated", "marketplace views for %s", name)
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

	installer.Status("Updated", "%d package(s) in %s scope", updated, scope.String())
	return nil
}

// updateGitHubPackage fetches the latest state of a GitHub package and
// updates the registry entry if the commit SHA has changed.
// Pinned packages (non-empty ref that isn't "HEAD") preserve their ref;
// unpinned packages resolve to the latest semver tag.
//
// If the store path is not a git repository (e.g. marketplace-style packages
// where only the plugin subdirectory was extracted during install), the update
// re-clones the repo into a temp directory and re-extracts the plugin.
func updateGitHubPackage(name string, entry registry.Entry, storePath string, paths installer.Paths, reg *registry.Registry) error {
	installer.Status("Checking", "%s for updates...", name)

	// Marketplace-style repos don't have .git/ in the store — re-clone.
	if _, err := os.Stat(filepath.Join(storePath, ".git")); os.IsNotExist(err) {
		return updateGitHubPackageViaClone(name, entry, storePath, paths, reg)
	}

	originalRef := entry.Source.Ref
	ref := originalRef
	isPinned := originalRef != "" && originalRef != "HEAD"

	if !isPinned {
		// Unpinned: resolve to latest (existing behavior).
		if err := git.FetchTags(storePath); err != nil {
			return err
		}
		latestRef, err := resolver.ResolveLatest(storePath)
		if err != nil {
			return err
		}
		ref = latestRef
		if ref != "HEAD" {
			if err := git.Checkout(storePath, ref); err != nil {
				return err
			}
		} else {
			if err := git.Pull(storePath); err != nil {
				return err
			}
		}
	} else {
		// Pinned: preserve the stored ref.
		// Use FetchTags (not Fetch) so tag refs are available for checkout.
		if err := git.FetchTags(storePath); err != nil {
			return err
		}
		if err := git.Checkout(storePath, ref); err != nil {
			return err
		}
		// If ref is a branch, pull to get latest commit.
		// If ref is a tag/SHA (detached HEAD), pull will fail — ignore that.
		_ = git.Pull(storePath)
	}

	newSHA, err := git.RevParseHEAD(storePath)
	if err != nil {
		return err
	}

	if newSHA == entry.Source.SHA {
		if isPinned {
			// Pinned package at its pinned ref — check if newer versions exist.
			latestRef, err := resolver.ResolveLatest(storePath)
			if err == nil && latestRef != ref && latestRef != "HEAD" {
				installer.Status("Pinned", "%s to %s (latest: %s)", name, ref, latestRef)
			} else {
				installer.Status("Pinned", "%s to %s", name, ref)
			}
		} else {
			installer.Status("Up-to-date", "%s is at latest", name)
		}
		return nil
	}

	manifests, _, err := manifest.LoadOrInfer(storePath)
	if err != nil {
		return err
	}
	m := manifests[0]

	if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
		return err
	}

	reg.Add(name, registry.Entry{
		Version: m.Version,
		Source: registry.Source{
			Type: "github",
			URL:  entry.Source.URL,
			Ref:  ref,
			SHA:  newSHA,
		},
		Platforms: entry.Platforms,
	})

	installer.Status("Updated", "%s → %s (%s)", name, m.Version, newSHA[:8])
	return nil
}

// updateGitHubPackageViaClone handles updates for packages whose store path
// is not a git repository (marketplace-style packages where install extracted
// only the plugin subdirectory). It re-clones the repo, resolves the ref,
// and replaces the store contents.
func updateGitHubPackageViaClone(name string, entry registry.Entry, storePath string, paths installer.Paths, reg *registry.Registry) error {
	tmpDir, err := os.MkdirTemp("", "summon-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, name)
	if err := git.Clone(entry.Source.URL, cloneDest); err != nil {
		return err
	}

	originalRef := entry.Source.Ref
	ref := originalRef
	isPinned := originalRef != "" && originalRef != "HEAD"

	if !isPinned {
		latestRef, err := resolver.ResolveLatest(cloneDest)
		if err != nil {
			return err
		}
		ref = latestRef
	}

	if ref != "HEAD" {
		if err := git.Checkout(cloneDest, ref); err != nil {
			return err
		}
	}

	newSHA, err := git.RevParseHEAD(cloneDest)
	if err != nil {
		return err
	}

	if newSHA == entry.Source.SHA {
		if isPinned {
			// Pinned package at its pinned ref — check if newer versions exist.
			latestRef, err := resolver.ResolveLatest(cloneDest)
			if err == nil && latestRef != ref && latestRef != "HEAD" {
				installer.Status("Pinned", "%s to %s (latest: %s)", name, ref, latestRef)
			} else {
				installer.Status("Pinned", "%s to %s", name, ref)
			}
		} else {
			installer.Status("Up-to-date", "%s is at latest", name)
		}
		return nil
	}

	manifests, pluginRoots, err := manifest.LoadOrInfer(cloneDest)
	if err != nil {
		return err
	}

	// Find the matching plugin by name.
	for i, m := range manifests {
		if m.Name == name {
			pluginRoot := pluginRoots[i]

			if err := os.RemoveAll(storePath); err != nil {
				return fmt.Errorf("removing old store entry: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
				return err
			}
			if err := os.Rename(pluginRoot, storePath); err != nil {
				return fmt.Errorf("moving to store: %w", err)
			}

			if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
				return err
			}

			reg.Add(name, registry.Entry{
				Version: m.Version,
				Source: registry.Source{
					Type: "github",
					URL:  entry.Source.URL,
					Ref:  ref,
					SHA:  newSHA,
				},
				Platforms: entry.Platforms,
			})

			installer.Status("Updated", "%s → %s (%s)", name, m.Version, newSHA[:8])
			return nil
		}
	}

	return fmt.Errorf("plugin %q not found in repository", name)
}
