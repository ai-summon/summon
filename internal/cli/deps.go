package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/depcheck"
	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/spf13/cobra"
)

var depsCmd = &cobra.Command{
	Use:   "deps <package>",
	Short: "Show dependencies of an installed package",
	Long:  "Displays the declared dependencies of a specific installed package, including version constraints, installed versions, and satisfaction status.",
	Example: `  summon deps code-reviewer
  summon deps prompt-library --json
  summon deps -g my-package`,
	Args: cobra.ExactArgs(1),
	RunE: runDeps,
}

var (
	depsGlobal  bool
	depsProject bool
	depsScope   string
	depsJSON    bool
)

func init() {
	depsCmd.Flags().BoolVarP(&depsGlobal, "global", "g", false, "Look up package in user scope")
	depsCmd.Flags().BoolVarP(&depsProject, "project", "p", false, "Look up package in project scope")
	depsCmd.Flags().StringVar(&depsScope, "scope", "", "Look up package at specific scope. One of: local, project, user")
	depsCmd.Flags().BoolVar(&depsJSON, "json", false, "Output results as JSON")
	depsCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
	rootCmd.AddCommand(depsCmd)
}

func runDeps(cmd *cobra.Command, args []string) error {
	name := args[0]
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Find the package's scope
	scope, err := resolveExistingPackageScope(projectDir, name, depsScope, depsGlobal, depsProject)
	if err != nil {
		return err
	}

	// Load the package's manifest from the store
	paths := installer.ResolvePaths(scope, projectDir)
	storePath := paths.StoreDir + "/" + name
	m, err := manifest.Load(storePath)
	if err != nil {
		return fmt.Errorf("loading manifest for %s: %w", name, err)
	}

	// Build registry view across all scopes for cross-scope satisfaction
	allScopes := platform.ScopePrecedence()
	registries := make(map[platform.Scope]*registry.Registry)
	for _, s := range allScopes {
		p := installer.ResolvePaths(s, projectDir)
		reg, loadErr := registry.Load(p.RegistryPath)
		if loadErr != nil {
			continue
		}
		registries[s] = reg
	}

	view := depcheck.NewRegistryView(registries)
	result := depcheck.CheckPackage(m, scope, view)

	if depsJSON {
		return printDepsJSON(result)
	}
	return printDepsHuman(result)
}

func printDepsJSON(result depcheck.PackageCheckResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func printDepsHuman(result depcheck.PackageCheckResult) error {
	if len(result.Results) == 0 {
		installer.Status("Info", "%s@%s (%s) has no dependencies",
			result.PackageName, result.Version, result.PackageScope)
		return nil
	}

	fmt.Fprintf(installer.Stdout, "   Dependencies for %s@%s (%s):\n\n",
		result.PackageName, result.Version, result.PackageScope)

	satisfied := 0
	for _, r := range result.Results {
		constraint := r.Constraint
		if constraint == "" {
			constraint = `""`
		}
		switch r.Status {
		case depcheck.Satisfied:
			satisfied++
			fmt.Fprintf(installer.Stdout, "     ✓ %s %s — satisfied by %s (%s)\n",
				r.DependencyName, constraint, r.InstalledVersion, r.InstalledScope)
		case depcheck.Missing:
			fmt.Fprintf(installer.Stdout, "     ✗ %s %s — not installed\n",
				r.DependencyName, constraint)
		case depcheck.VersionMismatch:
			fmt.Fprintf(installer.Stdout, "     ✗ %s %s — installed %s (%s), does not satisfy\n",
				r.DependencyName, constraint, r.InstalledVersion, r.InstalledScope)
		case depcheck.UnparseableConstraint:
			fmt.Fprintf(installer.Stdout, "     ✗ %s %s — %s\n",
				r.DependencyName, constraint, r.Message)
		}
	}

	fmt.Fprintf(installer.Stdout, "\n   %d of %d dependencies satisfied\n",
		satisfied, len(result.Results))
	return nil
}
