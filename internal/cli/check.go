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

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check dependency health of installed packages",
	Long:  "Validates that all installed packages have their declared dependencies satisfied. Reports missing and version-incompatible dependencies across all scopes.",
	Example: `  summon check
  summon check --scope user
  summon check -g
  summon check --json`,
	Args: cobra.NoArgs,
	RunE: runCheck,
}

var (
	checkGlobal  bool
	checkProject bool
	checkScope   string
	checkJSON    bool
)

func init() {
	checkCmd.Flags().BoolVarP(&checkGlobal, "global", "g", false, "Check user-scope packages only")
	checkCmd.Flags().BoolVarP(&checkProject, "project", "p", false, "Check project-scope packages only")
	checkCmd.Flags().StringVar(&checkScope, "scope", "", "Check packages at specific scope only. One of: local, project, user")
	checkCmd.Flags().BoolVar(&checkJSON, "json", false, "Output results as JSON")
	checkCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scopes, err := resolveQueryScopes(checkScope, checkGlobal, checkProject)
	if err != nil {
		return err
	}

	// Load registries for all scopes (need full view for cross-scope satisfaction)
	allScopes := platform.ScopePrecedence()
	registries := make(map[platform.Scope]*registry.Registry)
	for _, scope := range allScopes {
		paths := installer.ResolvePaths(scope, projectDir)
		reg, err := registry.Load(paths.RegistryPath)
		if err != nil {
			continue // Registry doesn't exist yet — skip
		}
		registries[scope] = reg
	}

	result := depcheck.CheckAll(
		scopes,
		registries,
		func(scope platform.Scope, name string) string {
			paths := installer.ResolvePaths(scope, projectDir)
			return paths.StoreDir + "/" + name
		},
		func(pkgDir string) (*manifest.Manifest, error) {
			return manifest.Load(pkgDir)
		},
	)

	if checkJSON {
		return printCheckJSON(result)
	}
	return printCheckHuman(result)
}

func printCheckJSON(result depcheck.CheckAllResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func printCheckHuman(result depcheck.CheckAllResult) error {
	if len(result.Packages) == 0 {
		installer.Status("Check", "no packages installed — nothing to check")
		return nil
	}

	installer.Status("Checking", "dependency health across all scopes...")
	fmt.Fprintln(installer.Stdout)

	unsatisfiedCount := 0
	for _, pkg := range result.Packages {
		if len(pkg.Results) == 0 {
			continue // No dependencies declared — skip from output
		}
		satisfied := 0
		for _, r := range pkg.Results {
			if r.Status == depcheck.Satisfied {
				satisfied++
			}
		}
		total := len(pkg.Results)

		if pkg.AllSatisfied {
			fmt.Fprintf(installer.Stdout, "     ✓ %s (%s) — all %d dependencies satisfied\n",
				pkg.PackageName, pkg.PackageScope, total)
		} else {
			unsatisfiedCount++
			fmt.Fprintf(installer.Stdout, "     ✗ %s (%s) — %d of %d dependencies unsatisfied\n",
				pkg.PackageName, pkg.PackageScope, total-satisfied, total)
			for _, r := range pkg.Results {
				if r.Status == depcheck.Satisfied {
					continue
				}
				switch r.Status {
				case depcheck.Missing:
					fmt.Fprintf(installer.Stdout, "         ✗ %s: not installed (install with: summon install %s)\n",
						r.DependencyName, r.DependencyName)
				case depcheck.VersionMismatch:
					fmt.Fprintf(installer.Stdout, "         ✗ %s: installed %s, requires %s\n",
						r.DependencyName, r.InstalledVersion, r.Constraint)
				case depcheck.UnparseableConstraint:
					fmt.Fprintf(installer.Stdout, "         ✗ %s: %s\n",
						r.DependencyName, r.Message)
				}
			}
		}
	}

	fmt.Fprintln(installer.Stdout)

	// Display circular dependency warnings
	for _, w := range result.Warnings {
		installer.StatusErr("Warning", "%s", w)
	}

	if unsatisfiedCount > 0 {
		installer.StatusErr("Result", "%d package(s) have unsatisfied dependencies", unsatisfiedCount)
		// Return a sentinel error so cobra sets non-zero exit code.
		return fmt.Errorf("%d package(s) have unsatisfied dependencies", unsatisfiedCount)
	}
	installer.Status("Result", "all dependencies satisfied")
	return nil
}
