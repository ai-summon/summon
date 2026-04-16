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

// pluginUpdateOutcome tracks the result of updating a single plugin on a single platform.
type pluginUpdateOutcome struct {
	name        string
	preVersion  string
	postVersion string
	err         error
}

// platformUpdateOutput groups all plugin update outcomes for a single platform.
type platformUpdateOutput struct {
	cli     string
	plugins []pluginUpdateOutcome
	newDeps []string // new dependency names installed on this platform
}

// updateResult tracks per-plugin summary across all platforms.
type updateResult struct {
	updated  int // plugins where version changed on at least one platform
	upToDate int // plugins where version stayed the same on all platforms
	failed   int // plugins that failed on all platforms
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

// updateTarget captures per-adapter metadata for a plugin to be updated.
type updateTarget struct {
	name       string
	scope      platform.Scope
	source     string // source identifier as reported by ListInstalled
	updateID   string // identifier to pass to adapter.Update
	preVersion string
}

// collectUpdateTargets discovers all plugins installed on each adapter and returns
// a map of adapter name → sorted list of update targets.
func collectUpdateTargets(names []string, adapters []platform.Adapter, defaultScope platform.Scope) map[string][]updateTarget {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	targets := make(map[string][]updateTarget)
	for _, a := range adapters {
		plugins, _ := a.ListInstalled(defaultScope)
		for _, p := range plugins {
			if !nameSet[p.Name] {
				continue
			}
			actualScope := defaultScope
			if p.Scope != "" {
				if parsed, err := platform.ParseScope(p.Scope); err == nil {
					actualScope = parsed
				}
			}
			updateID := p.Name
			if p.Source != "" {
				updateID = p.Source
			}
			targets[a.Name()] = append(targets[a.Name()], updateTarget{
				name:       p.Name,
				scope:      actualScope,
				source:     p.Source,
				updateID:   updateID,
				preVersion: p.Version,
			})
		}
	}

	// Fill in missing pre-update versions from plugin.json on disk
	for _, a := range adapters {
		for i, t := range targets[a.Name()] {
			if t.preVersion == "" {
				if dir, err := a.FindPluginDir(t.name, t.scope); err == nil {
					targets[a.Name()][i].preVersion = readPluginVersion(dir)
				}
			}
		}
	}

	return targets
}

// executeUpdates runs the actual update operations and collects results per platform.
func executeUpdates(adapters []platform.Adapter, targets map[string][]updateTarget) []platformUpdateOutput {
	var outputs []platformUpdateOutput
	for _, a := range adapters {
		adapterTargets := targets[a.Name()]
		if len(adapterTargets) == 0 {
			continue
		}

		output := platformUpdateOutput{cli: a.Name()}
		for _, t := range adapterTargets {
			outcome := pluginUpdateOutcome{name: t.name, preVersion: t.preVersion}
			if err := a.Update(t.updateID, t.scope); err != nil {
				outcome.err = err
			} else {
				outcome.postVersion = getPluginVersion(a, t.name, t.scope)
			}
			output.plugins = append(output.plugins, outcome)
		}
		outputs = append(outputs, output)
	}
	return outputs
}

// installNewDeps checks for new dependencies introduced by updated plugins and installs them.
func installNewDeps(adapters []platform.Adapter, targets map[string][]updateTarget, defaultScope platform.Scope, fetcher manifest.ManifestFetcher, outputs []platformUpdateOutput, stderr io.Writer) []platformUpdateOutput {
	if fetcher == nil {
		return outputs
	}

	// Collect all sources from successfully updated plugins
	sourcesByPlugin := make(map[string]string)
	for _, a := range adapters {
		for _, t := range targets[a.Name()] {
			if t.source != "" {
				if _, exists := sourcesByPlugin[t.name]; !exists {
					sourcesByPlugin[t.name] = t.source
				}
			}
		}
	}

	// For each source, fetch manifest and find new deps
	type newDep struct {
		specifier string
		name      string
	}
	allNewDeps := make(map[string]newDep) // deduped by dep name
	for _, source := range sourcesByPlugin {
		m, _ := fetcher.FetchManifest(source)
		if m == nil || len(m.Dependencies) == 0 {
			continue
		}
		for _, dep := range m.Dependencies {
			depName, _ := resolveDepName(dep)
			if depName != "" {
				allNewDeps[depName] = newDep{specifier: dep, name: depName}
			}
		}
	}

	if len(allNewDeps) == 0 {
		return outputs
	}

	// Build per-platform installed sets
	installedPerPlatform := make(map[string]map[string]bool)
	for _, a := range adapters {
		installed := make(map[string]bool)
		plugins, _ := a.ListInstalled(defaultScope)
		for _, p := range plugins {
			installed[p.Name] = true
		}
		installedPerPlatform[a.Name()] = installed
	}

	// Install missing deps per platform
	outputMap := make(map[string]*platformUpdateOutput)
	for i := range outputs {
		outputMap[outputs[i].cli] = &outputs[i]
	}

	// Sort dep names for deterministic output
	depNames := make([]string, 0, len(allNewDeps))
	for name := range allNewDeps {
		depNames = append(depNames, name)
	}
	sort.Strings(depNames)

	for _, a := range adapters {
		installed := installedPerPlatform[a.Name()]
		for _, depName := range depNames {
			if installed[depName] {
				continue
			}
			dep := allNewDeps[depName]
			if err := a.Install(dep.specifier, defaultScope); err != nil {
				fmt.Fprintf(stderr, "Warning: %s: failed to install new dependency %s: %v\n", a.Name(), dep.name, err)
				continue
			}
			out, ok := outputMap[a.Name()]
			if !ok {
				// Platform had no direct updates but needs new deps
				newOut := platformUpdateOutput{cli: a.Name()}
				outputs = append(outputs, newOut)
				outputMap[a.Name()] = &outputs[len(outputs)-1]
				out = outputMap[a.Name()]
			}
			out.newDeps = append(out.newDeps, dep.name)
		}
	}

	return outputs
}

// computePluginSummary derives per-plugin summary counters from per-platform outcomes.
func computePluginSummary(pluginNames []string, outputs []platformUpdateOutput) *updateResult {
	type pluginStatus struct {
		anyUpdated  bool
		anyUpToDate bool
		allFailed   bool
		seenAny     bool
	}

	statusMap := make(map[string]*pluginStatus)
	for _, name := range pluginNames {
		statusMap[name] = &pluginStatus{allFailed: true}
	}

	for _, out := range outputs {
		for _, p := range out.plugins {
			s := statusMap[p.name]
			if s == nil {
				continue
			}
			s.seenAny = true
			if p.err != nil {
				continue
			}
			s.allFailed = false
			if p.preVersion != "" && p.postVersion != "" && p.preVersion != p.postVersion {
				s.anyUpdated = true
			} else if p.preVersion != "" && p.postVersion != "" && p.preVersion == p.postVersion {
				s.anyUpToDate = true
			} else {
				// No version info → treat as updated
				s.anyUpdated = true
			}
		}
	}

	result := &updateResult{}
	for _, s := range statusMap {
		if !s.seenAny || s.allFailed {
			result.failed++
		} else if s.anyUpdated {
			result.updated++
		} else if s.anyUpToDate {
			result.upToDate++
		}
	}
	return result
}

// renderUpdateOutputs prints platform-first update results.
func renderUpdateOutputs(outputs []platformUpdateOutput, noColor bool, out io.Writer) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dimStyle := lipgloss.NewStyle().Faint(true)
	if noColor {
		headerStyle = lipgloss.NewStyle()
		checkStyle = lipgloss.NewStyle()
		errorStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	// Sort platforms for deterministic output
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].cli < outputs[j].cli
	})

	// Find max plugin name length across all platforms for alignment
	maxNameLen := 0
	for _, o := range outputs {
		for _, p := range o.plugins {
			if len(p.name) > maxNameLen {
				maxNameLen = len(p.name)
			}
		}
		for _, d := range o.newDeps {
			if len(d) > maxNameLen {
				maxNameLen = len(d)
			}
		}
	}

	for _, o := range outputs {
		fmt.Fprintf(out, "\n%s\n", headerStyle.Render(o.cli+":"))

		for _, p := range o.plugins {
			padding := strings.Repeat(" ", maxNameLen-len(p.name)+2)
			if p.err != nil {
				fmt.Fprintf(out, "  %s %s%s%s\n", errorStyle.Render("✗"), p.name, padding, dimStyle.Render("failed: "+p.err.Error()))
			} else if p.preVersion != "" && p.postVersion != "" && p.preVersion == p.postVersion {
				fmt.Fprintf(out, "  %s %s%s%s\n", dimStyle.Render("–"), p.name, padding, dimStyle.Render("up to date (v"+p.postVersion+")"))
			} else if p.preVersion != "" && p.postVersion != "" && p.preVersion != p.postVersion {
				fmt.Fprintf(out, "  %s %s%s%s\n", checkStyle.Render("✓"), p.name, padding, dimStyle.Render("v"+p.preVersion+" → v"+p.postVersion))
			} else {
				fmt.Fprintf(out, "  %s %s%s%s\n", checkStyle.Render("✓"), p.name, padding, dimStyle.Render("updated"))
			}
		}

		for i, dep := range o.newDeps {
			connector := "├──"
			if i == len(o.newDeps)-1 {
				connector = "└──"
			}
			depPadding := strings.Repeat(" ", maxNameLen-len(dep)+2)
			fmt.Fprintf(out, "  %s %s%s%s\n", dimStyle.Render(connector), dep, depPadding, dimStyle.Render("installed (new dependency)"))
		}
	}
}

func runUpdate(name string, deps *updateDeps) error {
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

	targets := collectUpdateTargets([]string{name}, adapters, scope)

	// Check if the plugin is installed on any platform
	found := false
	for _, ts := range targets {
		if len(ts) > 0 {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("package %q is not installed", name)
	}

	outputs := executeUpdates(adapters, targets)
	outputs = installNewDeps(adapters, targets, scope, deps.fetcher, outputs, deps.stderr)
	renderUpdateOutputs(outputs, deps.noColor, deps.stdout)

	// Check if all platforms failed
	allFailed := true
	for _, o := range outputs {
		for _, p := range o.plugins {
			if p.err == nil {
				allFailed = false
				break
			}
		}
		if !allFailed {
			break
		}
	}
	if allFailed {
		var errParts []string
		for _, o := range outputs {
			for _, p := range o.plugins {
				if p.err != nil {
					errParts = append(errParts, fmt.Sprintf("%s: %v", o.cli, p.err))
				}
			}
		}
		return fmt.Errorf("update failed on all platforms: %s", strings.Join(errParts, "; "))
	}

	return nil
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

	// Get all installed plugin names
	pluginMap := make(map[string]bool)
	for _, a := range adapters {
		plugins, _ := a.ListInstalled(scope)
		for _, p := range plugins {
			pluginMap[p.Name] = true
		}
	}

	if len(pluginMap) == 0 {
		fmt.Fprintln(out, "No plugins installed.")
		return nil
	}

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

	targets := collectUpdateTargets(names, adapters, scope)
	outputs := executeUpdates(adapters, targets)
	outputs = installNewDeps(adapters, targets, scope, deps.fetcher, outputs, deps.stderr)
	renderUpdateOutputs(outputs, deps.noColor, out)

	// Compute per-plugin summary
	result := computePluginSummary(names, outputs)

	var parts []string
	if result.updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", result.updated))
	}
	if result.upToDate > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", result.upToDate))
	}
	if result.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", result.failed))
	}
	if len(parts) > 0 {
		fmt.Fprintf(out, "\n%s\n", dimStyle.Render(strings.Join(parts, ", ")))
	}

	return nil
}
