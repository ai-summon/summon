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

// updateResult tracks per-plugin update outcome across adapters.
type updateResult struct {
	updated  int // adapters where version actually changed
	upToDate int // adapters where version stayed the same
	failed   int // adapters that errored
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
			_, err := runUpdate(args[0], deps)
			return err
		}
		return runUpdateAll(deps)
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateYes, "yes", false, "Skip confirmation prompts")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(name string, deps *updateDeps) (*updateResult, error) {
	out := deps.stdout
	result := &updateResult{}
	scope, err := platform.ParseScope(installScope)
	if err != nil {
		return nil, err
	}

	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return nil, fmt.Errorf("no supported CLIs detected")
	}
	adapters, err = platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return nil, err
	}

	// Find the plugin and capture per-adapter scopes, sources, and pre-update versions
	var source string
	pluginScopes := make(map[string]platform.Scope)
	pluginSources := make(map[string]string)
	preVersions := make(map[string]string)
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
				preVersions[a.Name()] = p.Version
			}
		}
	}
	if source == "" {
		return nil, fmt.Errorf("package %q is not installed", name)
	}

	// Fill in missing pre-update versions from plugin.json on disk
	for _, a := range adapters {
		adapterScope, ok := pluginScopes[a.Name()]
		if !ok {
			continue
		}
		if preVersions[a.Name()] == "" {
			if dir, err := a.FindPluginDir(name, adapterScope); err == nil {
				preVersions[a.Name()] = readPluginVersion(dir)
			}
		}
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
			result.failed++
		} else {
			preVersion := preVersions[a.Name()]
			postVersion := getPluginVersion(a, name, adapterScope)

			if preVersion != "" && postVersion != "" && preVersion == postVersion {
				fmt.Fprintf(out, "  %s %s%s%s\n", dimStyle.Render("–"), a.Name(), padding, dimStyle.Render("up to date (v"+postVersion+")"))
				result.upToDate++
			} else if preVersion != "" && postVersion != "" && preVersion != postVersion {
				fmt.Fprintf(out, "  %s %s%s%s\n", checkStyle.Render("✓"), a.Name(), padding, dimStyle.Render("v"+preVersion+" → v"+postVersion))
				result.updated++
			} else {
				fmt.Fprintf(out, "  %s %s%s%s\n", checkStyle.Render("✓"), a.Name(), padding, dimStyle.Render("updated"))
				result.updated++
			}
			updatedOn = append(updatedOn, a.Name())
		}
	}

	if len(updateErrors) > 0 && len(updatedOn) == 0 {
		return result, fmt.Errorf("update failed on all platforms: %s", strings.Join(updateErrors, "; "))
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
							return nil, fmt.Errorf("failed to install new dependency %s: %w", dep, err)
						}
					}
					depPadding := strings.Repeat(" ", maxDepLen-len(depName)+2)
					fmt.Fprintf(out, "  %s %s%s%s\n", dimStyle.Render(connector), depName, depPadding, dimStyle.Render("installed (new dependency)"))
				}
			}
		}
	}

	return result, nil
}

// getPluginVersion retrieves the current version of a plugin from the adapter.
// It first checks ListInstalled, then falls back to reading plugin.json on disk.
func getPluginVersion(a platform.Adapter, name string, scope platform.Scope) string {
	if plugins, err := a.ListInstalled(scope); err == nil {
		for _, p := range plugins {
			if p.Name == name && p.Version != "" {
				return p.Version
			}
		}
	}
	if dir, err := a.FindPluginDir(name, scope); err == nil {
		if v := readPluginVersion(dir); v != "" {
			return v
		}
	}
	return ""
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

	totalUpdated := 0
	totalUpToDate := 0
	totalFailed := 0
	for _, name := range names {
		result, err := runUpdate(name, deps)
		if err != nil {
			fmt.Fprintf(deps.stderr, "Warning: failed to update %s: %v\n", name, err)
			totalFailed++
		} else if result != nil {
			if result.updated > 0 {
				totalUpdated++
			} else if result.upToDate > 0 {
				totalUpToDate++
			}
		}
	}

	// Print summary
	var parts []string
	if totalUpdated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", totalUpdated))
	}
	if totalUpToDate > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", totalUpToDate))
	}
	if totalFailed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", totalFailed))
	}
	if len(parts) > 0 {
		fmt.Fprintf(out, "\n%s\n", dimStyle.Render(strings.Join(parts, ", ")))
	}

	return nil
}
