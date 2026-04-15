package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var updateYes bool

type updateDeps struct {
	runner  platform.CommandRunner
	fetcher manifest.ManifestFetcher
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	noColor bool
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

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dimStyle := lipgloss.NewStyle().Faint(true)
	if deps.noColor {
		headerStyle = lipgloss.NewStyle()
		checkStyle = lipgloss.NewStyle()
		errorStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	fmt.Fprintf(out, "\n%s\n", headerStyle.Render(name+":"))

	// Compute max platform name length for column alignment
	maxPlatformLen := 0
	for _, a := range adapters {
		if _, ok := pluginScopes[a.Name()]; ok {
			if len(a.Name()) > maxPlatformLen {
				maxPlatformLen = len(a.Name())
			}
		}
	}

	// Update via each adapter
	var updatedOn []string
	var updateErrors []string
	for _, a := range adapters {
		adapterScope, ok := pluginScopes[a.Name()]
		if !ok {
			continue
		}
		updateID := name
		if src := pluginSources[a.Name()]; src != "" {
			updateID = src
		}
		padding := strings.Repeat(" ", maxPlatformLen-len(a.Name())+2)
		if err := a.Update(updateID, adapterScope); err != nil {
			fmt.Fprintf(out, "  %s %s%s%s\n", errorStyle.Render("✗"), a.Name(), padding, dimStyle.Render("failed: "+err.Error()))
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", a.Name(), err))
		} else {
			fmt.Fprintf(out, "  %s %s%s%s\n", checkStyle.Render("✓"), a.Name(), padding, dimStyle.Render("updated"))
			updatedOn = append(updatedOn, a.Name())
		}
	}

	if len(updateErrors) > 0 && len(updatedOn) == 0 {
		return fmt.Errorf("update failed on all platforms: %s", strings.Join(updateErrors, "; "))
	}

	// Check for new dependencies
	if source != "" && deps.fetcher != nil {
		m, _ := deps.fetcher.FetchManifest(source)
		if m != nil && len(m.Dependencies) > 0 {
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
				maxDepLen := 0
				for _, dep := range newDeps {
					depName, _ := resolveDepName(dep)
					if len(depName) > maxDepLen {
						maxDepLen = len(depName)
					}
				}

				for i, dep := range newDeps {
					depName, _ := resolveDepName(dep)
					connector := "├──"
					if i == len(newDeps)-1 {
						connector = "└──"
					}
					for _, a := range adapters {
						if err := a.Install(dep, scope); err != nil {
							return fmt.Errorf("failed to install new dependency %s: %w", dep, err)
						}
					}
					depPadding := strings.Repeat(" ", maxDepLen-len(depName)+2)
					fmt.Fprintf(out, "  %s %s%s%s\n", dimStyle.Render(connector), depName, depPadding, dimStyle.Render("installed (new dependency)"))
				}
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

	// Sort plugin names for deterministic output
	names := make([]string, 0, len(pluginMap))
	for name := range pluginMap {
		names = append(names, name)
	}
	sort.Strings(names)

	dimStyle := lipgloss.NewStyle().Faint(true)
	if deps.noColor {
		dimStyle = lipgloss.NewStyle()
	}

	fmt.Fprintf(out, "\n%s\n", dimStyle.Render(fmt.Sprintf("Updating %d plugins...", len(pluginMap))))
	for _, name := range names {
		if err := runUpdate(name, deps); err != nil {
			fmt.Fprintf(deps.stderr, "Warning: failed to update %s: %v\n", name, err)
		}
	}

	return nil
}
