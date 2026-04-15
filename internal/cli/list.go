package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/spf13/cobra"
)

var listJSON bool

type listDeps struct {
	runner   platform.CommandRunner
	fetcher  manifest.ManifestFetcher
	adapters []platform.Adapter // if non-nil, use instead of auto-detecting
	stdout   io.Writer
}

func defaultListDeps() *listDeps {
	return &listDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewLocalManifestFetcher(),
		stdout:  os.Stdout,
	}
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins with dependency tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultListDeps()
		return runList(deps)
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(listCmd)
}

type listOutput struct {
	CLI     string       `json:"cli"`
	Plugins []listPlugin `json:"plugins"`
}

type listPlugin struct {
	Name         string   `json:"name"`
	Version      string   `json:"version,omitempty"`
	Dependencies []string `json:"dependencies"`
}

func runList(deps *listDeps) error {
	out := deps.stdout
	scope, _ := platform.ParseScope(installScope)

	var adapters []platform.Adapter
	if deps.adapters != nil {
		adapters = deps.adapters
	} else {
		adapters = platform.DetectAdapters(deps.runner)
	}
	if len(adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}
	adapters, err := platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return err
	}

	var outputs []listOutput

	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}

		output := listOutput{CLI: a.Name()}
		for _, p := range plugins {
			lp := listPlugin{Name: p.Name, Version: p.Version}

			// Read manifest from local plugin directory for dependency info and version fallback
			if deps.fetcher != nil {
				actualScope := pluginScope(p, scope)
				pluginDir, err := a.FindPluginDir(p.Name, actualScope)
				if err == nil {
					m, _ := deps.fetcher.FetchManifest(pluginDir)
					if m != nil {
						for _, dep := range m.Dependencies {
							depName, _ := resolveDepName(dep)
							if depName != "" {
								lp.Dependencies = append(lp.Dependencies, depName)
							}
						}
					}
					// Read version from plugin.json if not already set by adapter
					if lp.Version == "" {
						lp.Version = readPluginVersion(pluginDir)
					}
				}
			}
			output.Plugins = append(output.Plugins, lp)
		}
		outputs = append(outputs, output)
	}

	if listJSON {
		result := make(map[string][]listPlugin)
		for _, o := range outputs {
			result[o.CLI] = o.Plugins
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}

	// Human-readable tree output
	fmt.Fprintln(out, "Installed plugins:")
	fmt.Fprintln(out)
	for _, o := range outputs {
		fmt.Fprintf(out, "  %s:\n", o.CLI)
		if len(o.Plugins) == 0 {
			fmt.Fprintln(out, "    (none)")
			continue
		}
		for _, p := range o.Plugins {
			if p.Version != "" {
				fmt.Fprintf(out, "    %s (v%s)\n", p.Name, p.Version)
			} else {
				fmt.Fprintf(out, "    %s\n", p.Name)
			}
			for _, dep := range p.Dependencies {
				fmt.Fprintf(out, "    └── %s\n", dep)
			}
		}
		fmt.Fprintln(out)
	}

	total := 0
	for _, o := range outputs {
		total += len(o.Plugins)
	}
	if total == 0 {
		fmt.Fprintln(out, "No plugins installed.")
	}

	return nil
}

// Suppress linter
var _ = strings.TrimSpace

// readPluginVersion reads the version from .claude-plugin/plugin.json in the plugin directory.
func readPluginVersion(pluginDir string) string {
	data, err := os.ReadFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.Version
}
