package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/syscheck"
	"github.com/spf13/cobra"
)

var checkJSON bool

type checkDeps struct {
	runner   platform.CommandRunner
	fetcher  manifest.ManifestFetcher
	adapters []platform.Adapter // if non-nil, use instead of auto-detecting
	stdout   io.Writer
}

func defaultCheckDeps() *checkDeps {
	return &checkDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewLocalManifestFetcher(),
		stdout:  os.Stdout,
	}
}

var checkCmd = &cobra.Command{
	Use:   "check [package]",
	Short: "Check installed plugin health",
	Long:  "Verify system and plugin dependencies for installed plugins.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultCheckDeps()
		if len(args) > 0 {
			return runCheckSingle(args[0], deps)
		}
		return runCheckAll(deps)
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(checkCmd)
}

type checkOutput struct {
	CLI     string        `json:"cli"`
	Results []checkResult `json:"results"`
}

type checkResult struct {
	Name       string      `json:"name"`
	PluginDeps []depStatus `json:"plugin_deps,omitempty"`
	SystemDeps []sysStatus `json:"system_deps,omitempty"`
	OK         bool        `json:"ok"`
}

type depStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Required  bool   `json:"required"`
}

type sysStatus struct {
	Name     string `json:"name"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
	Optional bool   `json:"optional"`
	Reason   string `json:"reason,omitempty"`
}

func resolveAdapters(deps *checkDeps) ([]platform.Adapter, error) {
	var adapters []platform.Adapter
	if deps.adapters != nil {
		adapters = deps.adapters
	} else {
		adapters = platform.DetectAdapters(deps.runner)
	}
	if len(adapters) == 0 {
		return nil, fmt.Errorf("no supported CLIs detected")
	}
	return platform.FilterByTarget(adapters, targetFlag)
}

func runCheckAll(deps *checkDeps) error {
	scope, _ := platform.ParseScope(installScope)
	adapters, err := resolveAdapters(deps)
	if err != nil {
		return err
	}

	var outputs []checkOutput
	hasIssues := false
	totalPlugins := 0

	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}

		// Build installed set for THIS platform only
		installed := make(map[string]platform.InstalledPlugin)
		for _, p := range plugins {
			installed[p.Name] = p
		}

		output := checkOutput{CLI: a.Name()}
		for _, p := range plugins {
			m := resolveLocalManifest(a, p, scope, deps.fetcher)
			result := checkPlugin(p, m, installed)
			output.Results = append(output.Results, result)
			if !result.OK {
				hasIssues = true
			}
		}
		outputs = append(outputs, output)
		totalPlugins += len(plugins)
	}

	if totalPlugins == 0 {
		if checkJSON {
			fmt.Fprintln(deps.stdout, "{}")
		} else {
			fmt.Fprintln(deps.stdout, "No plugins installed.")
		}
		return nil
	}

	printCheckOutputs(deps.stdout, outputs)

	if hasIssues {
		return fmt.Errorf("health check failed: required dependencies missing")
	}
	return nil
}

func runCheckSingle(name string, deps *checkDeps) error {
	scope, _ := platform.ParseScope(installScope)
	adapters, err := resolveAdapters(deps)
	if err != nil {
		return err
	}

	var outputs []checkOutput
	hasIssues := false
	found := false

	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}

		// Build installed set for THIS platform only
		installed := make(map[string]platform.InstalledPlugin)
		var target *platform.InstalledPlugin
		for _, p := range plugins {
			installed[p.Name] = p
			if p.Name == name {
				cp := p
				target = &cp
			}
		}

		if target == nil {
			continue
		}
		found = true

		m := resolveLocalManifest(a, *target, scope, deps.fetcher)
		result := checkPlugin(*target, m, installed)
		outputs = append(outputs, checkOutput{
			CLI:     a.Name(),
			Results: []checkResult{result},
		})
		if !result.OK {
			hasIssues = true
		}
	}

	if !found {
		return fmt.Errorf("package %q is not installed", name)
	}

	printCheckOutputs(deps.stdout, outputs)

	if hasIssues {
		return fmt.Errorf("health check failed for %s", name)
	}
	return nil
}

// resolveLocalManifest reads a plugin's manifest from its local install directory.
func resolveLocalManifest(a platform.Adapter, p platform.InstalledPlugin, defaultScope platform.Scope, fetcher manifest.ManifestFetcher) *manifest.Manifest {
	if fetcher == nil {
		return nil
	}
	actualScope := pluginScope(p, defaultScope)
	pluginDir, err := a.FindPluginDir(p.Name, actualScope)
	if err != nil {
		return nil
	}
	m, _ := fetcher.FetchManifest(pluginDir)
	return m
}

func checkPlugin(p platform.InstalledPlugin, m *manifest.Manifest, installed map[string]platform.InstalledPlugin) checkResult {
	result := checkResult{Name: p.Name, OK: true}

	if m == nil {
		return result
	}

	// Check plugin dependencies
	for _, dep := range m.Dependencies {
		depName, _ := resolveDepName(dep)
		if depName == "" {
			continue
		}
		_, isInstalled := installed[depName]
		result.PluginDeps = append(result.PluginDeps, depStatus{
			Name:      depName,
			Installed: isInstalled,
			Required:  true,
		})
		if !isInstalled {
			result.OK = false
		}
	}

	// Check system requirements
	for _, sr := range m.SystemRequirements {
		req := syscheck.RequirementInput{
			Name:     sr.Name,
			Optional: sr.Optional,
			Reason:   sr.Reason,
		}
		checkResult := syscheck.Check([]syscheck.RequirementInput{req}, nil)
		if len(checkResult.Requirements) > 0 {
			r := checkResult.Requirements[0]
			result.SystemDeps = append(result.SystemDeps, sysStatus{
				Name:     r.Name,
				Found:    r.Found,
				Path:     r.Path,
				Optional: r.Optional,
				Reason:   r.Reason,
			})
			if !r.Found && !r.Optional {
				result.OK = false
			}
		}
	}

	return result
}

func printCheckOutputs(w io.Writer, outputs []checkOutput) {
	if checkJSON {
		result := make(map[string][]checkResult)
		for _, o := range outputs {
			result[o.CLI] = o.Results
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(w, string(data))
		return
	}

	fmt.Fprintln(w, "Checking installed plugins...")
	fmt.Fprintln(w)
	for _, o := range outputs {
		fmt.Fprintf(w, "%s:\n", o.CLI)
		if len(o.Results) == 0 {
			fmt.Fprintln(w, "  (none)")
		} else {
			for i, r := range o.Results {
				printCheckResult(w, r)
				if i < len(o.Results)-1 {
					fmt.Fprintln(w)
				}
			}
		}
		fmt.Fprintln(w)
	}
}

func printCheckResult(w io.Writer, r checkResult) {
	fmt.Fprintf(w, "  %s:", r.Name)
	if len(r.PluginDeps) == 0 && len(r.SystemDeps) == 0 {
		fmt.Fprintln(w, " ✓ no dependencies")
		return
	}
	fmt.Fprintln(w)
	if len(r.PluginDeps) > 0 {
		fmt.Fprintln(w, "    Plugin deps:")
		for _, d := range r.PluginDeps {
			if d.Installed {
				fmt.Fprintf(w, "      ✓ %s (installed)\n", d.Name)
			} else {
				fmt.Fprintf(w, "      ✗ %s (NOT installed) [required]\n", d.Name)
			}
		}
	}
	if len(r.SystemDeps) > 0 {
		fmt.Fprintln(w, "    System deps:")
		for _, s := range r.SystemDeps {
			if s.Found {
				fmt.Fprintf(w, "      ✓ %s (found: %s)\n", s.Name, s.Path)
			} else if s.Optional {
				fmt.Fprintf(w, "      ✗ %s (not found) [recommended: %s]\n", s.Name, s.Reason)
			} else {
				fmt.Fprintf(w, "      ✗ %s (not found) [required]\n", s.Name)
			}
		}
	}
}
