package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var listJSON bool

type listDeps struct {
	runner   platform.CommandRunner
	fetcher  manifest.ManifestFetcher
	adapters []platform.Adapter // if non-nil, use instead of auto-detecting
	stdout   io.Writer
	stderr   io.Writer
	noColor  bool
}

func defaultListDeps() *listDeps {
	return &listDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewLocalManifestFetcher(),
		stdout:  os.Stdout,
		stderr:  os.Stderr,
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

	adapters, err := resolveEnabledAdapters(&adapterResolverDeps{
		runner:   deps.runner,
		adapters: deps.adapters,
		target:   targetFlag,
		stderr:   deps.stderr,
	})
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
		sort.Slice(output.Plugins, func(i, j int) bool {
			return output.Plugins[i].Name < output.Plugins[j].Name
		})
		outputs = append(outputs, output)
	}

	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].CLI < outputs[j].CLI
	})

	if listJSON {
		result := make(map[string][]listPlugin)
		for _, o := range outputs {
			result[o.CLI] = o.Plugins
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintln(out)

	// Human-readable styled output
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	dimStyle := lipgloss.NewStyle().Faint(true)
	if deps.noColor {
		headerStyle = lipgloss.NewStyle()
		checkStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	// Find the longest plugin name for column alignment
	maxNameLen := 0
	for _, o := range outputs {
		for _, p := range o.Plugins {
			if len(p.Name) > maxNameLen {
				maxNameLen = len(p.Name)
			}
		}
	}

	for _, o := range outputs {
		fmt.Fprintf(out, "%s\n", headerStyle.Render(o.CLI+":"))
		if len(o.Plugins) == 0 {
			fmt.Fprintln(out, "  (none)")
			fmt.Fprintln(out)
			continue
		}
		for _, p := range o.Plugins {
			check := checkStyle.Render("✓")
			if p.Version != "" {
				padding := strings.Repeat(" ", maxNameLen-len(p.Name)+2)
				version := dimStyle.Render("v" + p.Version)
				fmt.Fprintf(out, "  %s %s%s%s\n", check, p.Name, padding, version)
			} else {
				fmt.Fprintf(out, "  %s %s\n", check, p.Name)
			}
			for _, dep := range p.Dependencies {
				fmt.Fprintf(out, "      └── %s\n", dimStyle.Render(dep))
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
