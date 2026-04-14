package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/store"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check health of installed packages",
	Long:  "Validates that all installed packages are present in the store and have valid platform registrations. Reports broken links and missing store entries.",
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

type checkResult struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
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

	activePlatforms := platform.DetectActive(projectDir)

	var results []checkResult
	brokenCount := 0

	for _, scope := range scopes {
		paths := installer.ResolvePaths(scope, projectDir)
		reg, err := registry.Load(paths.RegistryPath)
		if err != nil {
			continue
		}

		s := store.New(paths.StoreDir)
		for name := range reg.Packages {
			r := checkResult{Name: name, Scope: scope.String()}
			if !s.Has(name) {
				r.Status = "missing"
				r.Message = "not found in store"
				brokenCount++
			} else if s.IsBrokenLink(name) {
				r.Status = "broken"
				r.Message = "broken symlink"
				brokenCount++
			} else {
				r.Status = "ok"
			}
			results = append(results, r)
		}
	}

	if checkJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	if len(results) == 0 {
		installer.Status("Check", "no packages installed — nothing to check")
		return nil
	}

	installer.Status("Checking", "package health...")
	fmt.Fprintln(installer.Stdout)
	for _, r := range results {
		switch r.Status {
		case "ok":
			fmt.Fprintf(installer.Stdout, "     ✓ %s (%s)\n", r.Name, r.Scope)
		default:
			fmt.Fprintf(installer.Stdout, "     ✗ %s (%s) — %s\n", r.Name, r.Scope, r.Message)
		}
	}
	fmt.Fprintln(installer.Stdout)

	if len(activePlatforms) == 0 {
		installer.StatusErr("Warning", "no AI platform detected")
	}

	if brokenCount > 0 {
		installer.StatusErr("Result", "%d package(s) have issues", brokenCount)
		return fmt.Errorf("%d package(s) have issues", brokenCount)
	}
	installer.Status("Result", "all %d packages healthy", len(results))
	return nil
}
