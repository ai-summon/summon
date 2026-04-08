package cli

import (
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install a package",
	Long:  "Install a package from the catalog, GitHub, or a local path. Run without arguments to restore from registry.yaml.",
	Example: `  summon install superpowers
  summon install github:obra/superpowers
  summon install --path ../superpowers
	summon install --scope project superpowers
  summon install -g superpowers
  summon install --ref v5.0.7 superpowers
  summon install`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

var (
	installPath    string
	installGlobal  bool
	installProject bool
	installScope   string
	installRef     string
	installForce   bool
)

func init() {
	installCmd.Flags().StringVar(&installPath, "path", "", "Install from local filesystem path")
	installCmd.Flags().BoolVarP(&installGlobal, "global", "g", false, "Shortcut for --scope user")
	installCmd.Flags().BoolVarP(&installProject, "project", "p", false, "Shortcut for --scope project")
	installCmd.Flags().StringVar(&installScope, "scope", "", "Installation target scope. One of local, project, user")
	installCmd.Flags().StringVar(&installRef, "ref", "", "Pin to specific git tag, branch, or commit")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Install even if no compatible platform is active")
	installCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
	rootCmd.AddCommand(installCmd)
}

func resolveInstallScope(scopeFlag string, global bool, project bool) (platform.Scope, error) {
	if scopeFlag != "" {
		return platform.ParseScope(scopeFlag)
	}
	if global {
		return platform.ScopeUser, nil
	}
	if project {
		return platform.ScopeProject, nil
	}
	return platform.ScopeLocal, nil
}

func resolveRestoreScope(scopeFlag string, global bool, project bool) (platform.Scope, error) {
	if scopeFlag != "" {
		return platform.ParseScope(scopeFlag)
	}
	if global {
		return platform.ScopeUser, nil
	}
	if project {
		return platform.ScopeProject, nil
	}
	return platform.ScopeProject, nil
}

// runInstall handles "summon install [package]".
// With no arguments it restores all packages recorded in registry.yaml.
// Otherwise it delegates to installer.Install which resolves the source
// (catalog name, github: prefix, or --path) and performs the clone/link.
func runInstall(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// No package specified → restore all previously-installed packages.
	if len(args) == 0 && installPath == "" {
		scope, err := resolveRestoreScope(installScope, installGlobal, installProject)
		if err != nil {
			return err
		}
		if scope == platform.ScopeLocal {
			return fmt.Errorf("local installs are not restored automatically; use --scope project or --scope user")
		}
		installer.Status("Restoring", "%s scope packages...", scope.String())
		return installer.RestoreScope(scope, projectDir)
	}

	scope, err := resolveInstallScope(installScope, installGlobal, installProject)
	if err != nil {
		return err
	}

	pkg := ""
	if len(args) > 0 {
		pkg = args[0]
	}

	opts := installer.Options{
		Package:       pkg,
		Path:          installPath,
		Ref:           installRef,
		Force:         installForce,
		Global:        installGlobal,
		Scope:         scope,
		ProjectDir:    projectDir,
		SummonVersion: version,
	}

	if err := installer.Install(opts); err != nil {
		return err
	}

	if scope != platform.ScopeUser {
		if gitErr := installer.EnsureGitignore(projectDir); gitErr != nil {
			installer.StatusErr("Warning", "could not update .gitignore: %v", gitErr)
		}
	}

	return nil
}
