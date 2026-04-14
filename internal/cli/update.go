package cli

import (
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/git"
	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/resolver"
	"github.com/ai-summon/summon/internal/store"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [package]",
	Short: "Update installed packages",
	Long:  "Update a specific package or all packages. Unpinned GitHub packages resolve to the latest tag; pinned packages refresh at their stored ref.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUpdate,
}

var updateGlobal bool
var updateProject bool
var updateScope string

func init() {
	updateCmd.Flags().BoolVarP(&updateGlobal, "global", "g", false, "Shortcut for --scope user")
	updateCmd.Flags().BoolVarP(&updateProject, "project", "p", false, "Shortcut for --scope project")
	updateCmd.Flags().StringVar(&updateScope, "scope", "", "Target scope. One of local, project, user")
	updateCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		scope, err := resolveExistingPackageScope(projectDir, args[0], updateScope, updateGlobal, updateProject)
		if err != nil {
			return err
		}
		return runScopedUpdate(projectDir, scope, args[0])
	}

	scope, err := resolveInstallScope(updateScope, updateGlobal, updateProject)
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

	activePlatforms := platform.DetectActive(projectDir)

	updated := 0
	for _, name := range toUpdate {
		entry, _ := reg.Get(name)
		storePath := s.PackagePath(name)

		switch entry.Source.Type {
		case "github":
			if err := updateGitHubPackage(name, entry, storePath, paths, reg, s, activePlatforms, scope); err != nil {
				installer.StatusErr("Error", "updating %s: %v", name, err)
				continue
			}
			updated++
		case "local":
			installer.Status("Up-to-date", "%s (local) — no update needed", name)
			updated++
		}
	}

	if err := reg.Save(paths.RegistryPath); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}

	installer.Status("Updated", "%d package(s) in %s scope", updated, scope.String())
	return nil
}

func updateGitHubPackage(name string, entry registry.Entry, storePath string, paths installer.Paths, reg *registry.Registry, s *store.Store, platforms []platform.Adapter, scope platform.Scope) error {
	installer.Status("Checking", "%s for updates...", name)

	originalRef := entry.Source.Ref
	ref := originalRef
	isPinned := originalRef != "" && originalRef != "HEAD"

	// Try in-place update if .git exists
	hasGitDir := false
	if info, err := os.Stat(storePath + "/.git"); err == nil && info.IsDir() {
		hasGitDir = true
	}

	if hasGitDir {
		if !isPinned {
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
			if err := git.FetchTags(storePath); err != nil {
				return err
			}
			if err := git.Checkout(storePath, ref); err != nil {
				return err
			}
			_ = git.Pull(storePath)
		}
	} else {
		// Re-clone if no .git directory
		tmpDir, err := installer.MakeScopedTempDir(paths, "summon-update-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		cloneDest := tmpDir + "/" + name
		if err := git.Clone(entry.Source.URL, cloneDest); err != nil {
			return err
		}

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

		if err := s.MoveFromStage(name, cloneDest); err != nil {
			return fmt.Errorf("moving to store: %w", err)
		}
		storePath = s.PackagePath(name)
	}

	newSHA, err := git.RevParseHEAD(storePath)
	if err != nil {
		return err
	}

	if newSHA == entry.Source.SHA {
		if isPinned {
			installer.Status("Pinned", "%s to %s", name, ref)
		} else {
			installer.Status("Up-to-date", "%s is at latest", name)
		}
		return nil
	}

	version := ref
	if version == "HEAD" {
		version = newSHA[:8]
	}

	reg.Add(name, registry.Entry{
		Version: version,
		Source: registry.Source{
			Type: "github",
			URL:  entry.Source.URL,
			Ref:  ref,
			SHA:  newSHA,
		},
		Platforms: entry.Platforms,
	})

	// Re-register with platforms
	for _, a := range platforms {
		if platform.SupportsScope(a, scope) {
			_ = a.DiscoverPackage(name, storePath, scope)
		}
	}

	installer.Status("Updated", "%s → %s (%s)", name, version, newSHA[:8])
	return nil
}
