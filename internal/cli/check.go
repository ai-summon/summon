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
	runner  platform.CommandRunner
	fetcher manifest.ManifestFetcher
	stdout  io.Writer
}

func defaultCheckDeps() *checkDeps {
	return &checkDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewRemoteFetcher(nil, &execGitRunner{}),
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

type checkResult struct {
	Name       string       `json:"name"`
	PluginDeps []depStatus  `json:"plugin_deps,omitempty"`
	SystemDeps []sysStatus  `json:"system_deps,omitempty"`
	OK         bool         `json:"ok"`
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

func runCheckAll(deps *checkDeps) error {
	scope, _ := platform.ParseScope(installScope)
	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}
	adapters, err := platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return err
	}

	// Collect unique plugins
	pluginMap := make(map[string]platform.InstalledPlugin)
	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			if _, exists := pluginMap[p.Name]; !exists {
				pluginMap[p.Name] = p
			}
		}
	}

	if len(pluginMap) == 0 {
		fmt.Fprintln(deps.stdout, "No plugins installed.")
		return nil
	}

	fmt.Fprintln(deps.stdout, "Checking all installed plugins...")
	fmt.Fprintln(deps.stdout)

	var results []checkResult
	hasIssues := false
	for _, p := range pluginMap {
		result := checkPlugin(p, pluginMap, deps)
		results = append(results, result)
		if !result.OK {
			hasIssues = true
		}
	}

	if checkJSON {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(deps.stdout, string(data))
	} else {
		for _, r := range results {
			printCheckResult(deps.stdout, r)
		}
	}

	if hasIssues {
		return fmt.Errorf("health check failed: required dependencies missing")
	}
	return nil
}

func runCheckSingle(name string, deps *checkDeps) error {
	scope, _ := platform.ParseScope(installScope)
	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}
	adapters, err := platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return err
	}

	// Find the plugin
	pluginMap := make(map[string]platform.InstalledPlugin)
	var target *platform.InstalledPlugin
	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			pluginMap[p.Name] = p
			if p.Name == name {
				cp := p
				target = &cp
			}
		}
	}

	if target == nil {
		return fmt.Errorf("package %q is not installed", name)
	}

	result := checkPlugin(*target, pluginMap, deps)

	if checkJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(deps.stdout, string(data))
	} else {
		printCheckResult(deps.stdout, result)
	}

	if !result.OK {
		return fmt.Errorf("health check failed for %s", name)
	}
	return nil
}

func checkPlugin(p platform.InstalledPlugin, installed map[string]platform.InstalledPlugin, deps *checkDeps) checkResult {
	result := checkResult{Name: p.Name, OK: true}

	if deps.fetcher == nil || p.Source == "" {
		return result
	}

	m, _ := deps.fetcher.FetchManifest(p.Source)
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

func printCheckResult(w io.Writer, r checkResult) {
	fmt.Fprintf(w, "%s:\n", r.Name)
	if len(r.PluginDeps) > 0 {
		fmt.Fprintln(w, "  Plugin deps:")
		for _, d := range r.PluginDeps {
			if d.Installed {
				fmt.Fprintf(w, "    ✓ %s (installed)\n", d.Name)
			} else {
				fmt.Fprintf(w, "    ✗ %s (NOT installed) [required]\n", d.Name)
			}
		}
	}
	if len(r.SystemDeps) > 0 {
		fmt.Fprintln(w, "  System deps:")
		for _, s := range r.SystemDeps {
			if s.Found {
				fmt.Fprintf(w, "    ✓ %s (found: %s)\n", s.Name, s.Path)
			} else if s.Optional {
				fmt.Fprintf(w, "    ✗ %s (not found) [recommended: %s]\n", s.Name, s.Reason)
			} else {
				fmt.Fprintf(w, "    ✗ %s (not found) [required]\n", s.Name)
			}
		}
	}
	fmt.Fprintln(w)
}
