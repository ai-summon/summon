package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/spf13/cobra"
)

var updateYes bool

type updateDeps struct {
	runner  platform.CommandRunner
	fetcher manifest.ManifestFetcher
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func defaultUpdateDeps() *updateDeps {
	return &updateDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewRemoteFetcher(nil, &execGitRunner{}),
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

var updateCmd = &cobra.Command{
	Use:   "update [package]",
	Short: "Update a plugin and resolve new dependencies",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultUpdateDeps()
		if len(args) > 0 {
			return runUpdate(args[0], deps)
		}
		return runUpdateAll(deps)
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateYes, "yes", false, "Skip confirmation prompts")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(name string, deps *updateDeps) error {
	out := deps.stdout
	scope, err := platform.ParseScope(installScope)
	if err != nil {
		return err
	}

	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}
	adapters, err = platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return err
	}

	// Find the plugin and capture per-adapter scopes and sources
	var source string
	pluginScopes := make(map[string]platform.Scope)
	pluginSources := make(map[string]string)
	for _, a := range adapters {
		plugins, _ := a.ListInstalled(scope)
		for _, p := range plugins {
			if p.Name == name {
				if source == "" {
					source = p.Source
				}
				actualScope := scope
				if p.Scope != "" {
					if parsed, err := platform.ParseScope(p.Scope); err == nil {
						actualScope = parsed
					}
				}
				pluginScopes[a.Name()] = actualScope
				pluginSources[a.Name()] = p.Source
			}
		}
	}
	if source == "" {
		return fmt.Errorf("package %q is not installed", name)
	}

	// Update via each adapter (best-effort: try all platforms, collect errors)
	fmt.Fprintf(out, "Updating %s...\n", name)
	var updatedOn []string
	var updateErrors []string
	for _, a := range adapters {
		adapterScope, ok := pluginScopes[a.Name()]
		if !ok {
			continue // plugin not installed on this adapter
		}
		updateID := name
		if src := pluginSources[a.Name()]; src != "" {
			updateID = src
		}
		if err := a.Update(updateID, adapterScope); err != nil {
			fmt.Fprintf(deps.stderr, "  ✗ update failed on %s: %v\n", a.Name(), err)
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", a.Name(), err))
		} else {
			updatedOn = append(updatedOn, a.Name())
		}
	}

	if len(updatedOn) > 0 {
		fmt.Fprintf(out, "  ✓ %s updated (%s)\n", name, strings.Join(updatedOn, ", "))
	}
	if len(updateErrors) > 0 && len(updatedOn) == 0 {
		return fmt.Errorf("update failed on all platforms: %s", strings.Join(updateErrors, "; "))
	}

	// Check for new dependencies
	if source != "" && deps.fetcher != nil {
		fmt.Fprintln(out, "\nChecking for new dependencies...")
		m, _ := deps.fetcher.FetchManifest(source)
		if m != nil && len(m.Dependencies) > 0 {
			// Get installed packages
			installed := make(map[string]bool)
			for _, a := range adapters {
				plugins, _ := a.ListInstalled(scope)
				for _, p := range plugins {
					installed[p.Name] = true
				}
			}

			var newDeps []string
			for _, dep := range m.Dependencies {
				depName, _ := resolveDepName(dep)
				if depName != "" && !installed[depName] {
					newDeps = append(newDeps, dep)
				}
			}

			if len(newDeps) > 0 {
				for _, dep := range newDeps {
					fmt.Fprintf(out, "  New dependency: %s (not installed)\n", dep)
				}
				// Install new deps using the install flow
				fmt.Fprintln(out, "\nInstalling new dependencies:")
				for _, dep := range newDeps {
					depName, _ := resolveDepName(dep)
					for _, a := range adapters {
						if err := a.Install(dep, scope); err != nil {
							return fmt.Errorf("failed to install new dependency %s: %w", dep, err)
						}
					}
					fmt.Fprintf(out, "  ✓ %s installed\n", depName)
				}
			} else {
				fmt.Fprintln(out, "  No new dependencies")
			}
		}
	}

	return nil
}

func runUpdateAll(deps *updateDeps) error {
	out := deps.stdout
	scope, err := platform.ParseScope(installScope)
	if err != nil {
		return err
	}

	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}
	adapters, err = platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return err
	}

	// Get all installed plugins
	pluginMap := make(map[string]string)
	for _, a := range adapters {
		plugins, _ := a.ListInstalled(scope)
		for _, p := range plugins {
			if _, exists := pluginMap[p.Name]; !exists {
				pluginMap[p.Name] = p.Source
			}
		}
	}

	if len(pluginMap) == 0 {
		fmt.Fprintln(out, "No plugins installed.")
		return nil
	}

	fmt.Fprintf(out, "Updating all %d installed plugins...\n\n", len(pluginMap))
	for name := range pluginMap {
		if err := runUpdate(name, deps); err != nil {
			fmt.Fprintf(deps.stderr, "Warning: failed to update %s: %v\n", name, err)
		}
	}

	return nil
}
