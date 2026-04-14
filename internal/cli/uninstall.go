package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ai-summon/summon/internal/depgraph"
	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/spf13/cobra"
)

var uninstallYes bool

type uninstallDeps struct {
	runner  platform.CommandRunner
	fetcher manifest.ManifestFetcher
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func defaultUninstallDeps() *uninstallDeps {
	return &uninstallDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewRemoteFetcher(nil, &execGitRunner{}),
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <package>",
	Short: "Uninstall a package with reverse dependency warnings",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultUninstallDeps()
		return runUninstall(args[0], deps)
	},
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallYes, "yes", false, "Skip confirmation prompts")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(name string, deps *uninstallDeps) error {
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

	// Verify the package is installed on at least one CLI
	type adapterScope struct {
		adapter platform.Adapter
		scope   platform.Scope
	}
	var installedOn []adapterScope
	seen := make(map[string]bool)
	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			if p.Name == name {
				actualScope := scope
				if p.Scope != "" {
					if parsed, err := platform.ParseScope(p.Scope); err == nil {
						actualScope = parsed
					}
				}
				key := a.Name() + ":" + string(actualScope)
				if !seen[key] {
					seen[key] = true
					installedOn = append(installedOn, adapterScope{adapter: a, scope: actualScope})
				}
			}
		}
	}

	if len(installedOn) == 0 {
		return fmt.Errorf("package %q is not installed on any detected CLI", name)
	}

	// Scan for reverse dependencies
	graph := depgraph.NewGraph()
	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			graph.AddNode(&depgraph.Package{Name: p.Name, Source: p.Source})
			// Fetch manifest to discover dependencies
			if p.Source != "" && deps.fetcher != nil {
				m, _ := deps.fetcher.FetchManifest(p.Source)
				if m != nil {
					for _, dep := range m.Dependencies {
						r, _ := resolveDepName(dep)
						if r != "" {
							graph.AddEdge(p.Name, r)
						}
					}
				}
			}
		}
	}

	reverseDeps := graph.ReverseDependencies()
	if dependents, ok := reverseDeps[name]; ok && len(dependents) > 0 {
		fmt.Fprintf(out, "⚠️  The following installed packages depend on %s:\n", name)
		for _, d := range dependents {
			fmt.Fprintf(out, "  • %s\n", d)
		}
		fmt.Fprintln(out, "\nUninstalling may break these packages.")

		if !uninstallYes {
			fmt.Fprintf(out, "Continue? [y/N]: ")
			if !confirmPrompt(deps.stdin) {
				return fmt.Errorf("uninstall cancelled")
			}
		}
	}

	// Delegate uninstall (best-effort: try all platforms, collect errors)
	fmt.Fprintln(out)
	var failed []string
	var succeeded []string
	for _, entry := range installedOn {
		if err := entry.adapter.Uninstall(name, entry.scope); err != nil {
			fmt.Fprintf(out, "  ✗ failed to uninstall %s from %s: %v\n", name, entry.adapter.Name(), err)
			failed = append(failed, fmt.Sprintf("%s: %v", entry.adapter.Name(), err))
		} else {
			fmt.Fprintf(out, "  ✓ %s uninstalled (%s)\n", name, entry.adapter.Name())
			succeeded = append(succeeded, entry.adapter.Name())
		}
	}

	if len(failed) > 0 {
		fmt.Fprintln(out)
		if len(succeeded) > 0 {
			fmt.Fprintf(out, "Partially uninstalled %s (succeeded: %s)\n", name, strings.Join(succeeded, ", "))
		}
		return fmt.Errorf("uninstall failed on %d platform(s): %s", len(failed), strings.Join(failed, "; "))
	}

	fmt.Fprintf(out, "\nUninstalled %s\n", name)
	return nil
}

func resolveDepName(dep string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(dep))
	if scanner.Scan() {
		d := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(d, "gh:") {
			parts := strings.SplitN(strings.TrimPrefix(d, "gh:"), "/", 2)
			if len(parts) == 2 {
				return strings.TrimSuffix(parts[1], ".git"), nil
			}
		}
		if strings.Contains(d, "@") {
			return strings.SplitN(d, "@", 2)[0], nil
		}
		if strings.HasPrefix(d, "https://") || strings.HasPrefix(d, "http://") {
			parts := strings.Split(strings.TrimRight(d, "/"), "/")
			return strings.TrimSuffix(parts[len(parts)-1], ".git"), nil
		}
		return d, nil
	}
	return "", fmt.Errorf("empty dep")
}
