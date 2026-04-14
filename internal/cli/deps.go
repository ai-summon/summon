package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/spf13/cobra"
)

var depsCmd = &cobra.Command{
	Use:   "deps <package>",
	Short: "Show details of an installed package",
	Long:  "Displays the details of a specific installed package including version, source, and platform information.",
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

	scope, err := resolveExistingPackageScope(projectDir, name, depsScope, depsGlobal, depsProject)
	if err != nil {
		return err
	}

	paths := installer.ResolvePaths(scope, projectDir)
	reg, err := registry.Load(paths.RegistryPath)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	entry, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("package %q not found in %s registry", name, scope.String())
	}

	info := map[string]interface{}{
		"name":      name,
		"version":   entry.Version,
		"scope":     scope.String(),
		"source":    entry.Source,
		"platforms": entry.Platforms,
	}

	if depsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	installer.Status("Package", "%s@%s (%s scope)", name, entry.Version, scope.String())
	fmt.Fprintf(installer.Stdout, "   Source: %s %s\n", entry.Source.Type, entry.Source.URL)
	if entry.Source.Ref != "" {
		fmt.Fprintf(installer.Stdout, "   Ref: %s\n", entry.Source.Ref)
	}
	if entry.Source.SHA != "" {
		fmt.Fprintf(installer.Stdout, "   SHA: %s\n", entry.Source.SHA)
	}
	if len(entry.Platforms) > 0 {
		fmt.Fprintf(installer.Stdout, "   Platforms: %v\n", entry.Platforms)
	}
	return nil
}
