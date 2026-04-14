package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/ai-summon/summon/internal/depgraph"
	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/resolver"
	"github.com/ai-summon/summon/internal/syscheck"
	"github.com/spf13/cobra"
)

var (
	installYes   bool
	installForce bool
	installScope string
)

// installDeps holds the injectable dependencies for the install command.
type installDeps struct {
	runner      platform.CommandRunner
	fetcher     manifest.ManifestFetcher
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
}

// defaultInstallDeps returns the production dependencies.
func defaultInstallDeps() *installDeps {
	return &installDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewRemoteFetcher(&http.Client{}, &execGitRunner{}),
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

var installCmd = &cobra.Command{
	Use:   "install <package>",
	Short: "Install a package with full dependency resolution",
	Long: `Install a package with full dependency resolution.
Resolves transitive dependencies, checks system prerequisites, and installs
across all detected CLIs (or a specific target with --target).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultInstallDeps()
		return runInstall(args[0], deps)
	},
}

func init() {
	installCmd.Flags().BoolVar(&installYes, "yes", false, "Skip preview confirmation (CI/CD mode)")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Bypass all system dependency checks")
	installCmd.Flags().StringVar(&installScope, "scope", "user", "Installation scope: user, project, local")
	rootCmd.AddCommand(installCmd)
}

func runInstall(specifier string, deps *installDeps) error {
	out := deps.stdout

	// 1. Parse the scope
	scope, err := platform.ParseScope(installScope)
	if err != nil {
		return err
	}

	// 2. Detect platform adapters
	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected. Install copilot or claude CLI first")
	}

	adapters, err = platform.FilterByTarget(adapters, targetFlag)
	if err != nil {
		return err
	}

	// Validate scope for all target adapters
	for _, a := range adapters {
		if err := platform.ValidateScope(a, scope); err != nil {
			return err
		}
	}

	// 3. Resolve the package source
	resolved, err := resolver.Resolve(specifier)
	if err != nil {
		return fmt.Errorf("failed to resolve package: %w", err)
	}

	installSource := specifier
	if resolved.Source != "" {
		installSource = resolved.Source
	}

	// 4. Fetch the manifest (if exists)
	fmt.Fprintln(out, "Resolving dependencies...")
	m, err := deps.fetcher.FetchManifest(installSource)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// 5. Build dependency graph if manifest exists
	graph := depgraph.NewGraph()
	graph.AddNode(&depgraph.Package{Name: resolved.Name, Source: installSource})

	if m != nil {
		// Resolve transitive dependencies
		if err := resolveTransitiveDeps(graph, resolved.Name, m, deps.fetcher); err != nil {
			return err
		}

		depLine := fmt.Sprintf("  %s", resolved.Name)
		if len(m.Dependencies) > 0 {
			depNames := make([]string, len(m.Dependencies))
			for i, d := range m.Dependencies {
				r, _ := resolver.Resolve(d)
				if r != nil {
					depNames[i] = r.Name
				} else {
					depNames[i] = d
				}
			}
			depLine += " → " + strings.Join(depNames, ", ")
		}
		fmt.Fprintln(out, depLine)
	}

	// 6. Detect cycles
	allPackages, err := graph.Resolve(resolved.Name)
	if err != nil {
		return err
	}

	// 7. Check system requirements (unless --force)
	if m != nil && len(m.SystemRequirements) > 0 && !installForce {
		fmt.Fprintln(out, "\nSystem requirements check:")
		reqs := make([]syscheck.RequirementInput, len(m.SystemRequirements))
		for i, sr := range m.SystemRequirements {
			reqs[i] = syscheck.RequirementInput{
				Name:     sr.Name,
				Optional: sr.Optional,
				Reason:   sr.Reason,
			}
		}

		result := syscheck.Check(reqs, nil)
		for _, r := range result.Requirements {
			fmt.Fprintln(out, syscheck.FormatCheck(r))
		}

		if result.HasRequired {
			if installYes {
				return fmt.Errorf("required system dependency missing (--yes mode halts on missing required deps)")
			}
			fmt.Fprintf(out, "\n⚠️  Required system dependencies are missing.\nContinue anyway? [y/N]: ")
			if !confirmPrompt(deps.stdin) {
				return fmt.Errorf("installation cancelled: missing required system dependencies")
			}
		}
	}

	// 8. Query installed packages and filter
	installed := make(map[string]bool)
	for _, a := range adapters {
		plugins, err := a.ListInstalled(scope)
		if err != nil {
			fmt.Fprintf(deps.stderr, "Warning: could not list installed plugins for %s: %v\n", a.Name(), err)
			continue
		}
		for _, p := range plugins {
			installed[p.Name] = true
		}
	}

	toInstall := depgraph.FilterInstalled(allPackages, installed)
	if len(toInstall) == 0 {
		fmt.Fprintln(out, "\nAll packages are already installed.")
		return nil
	}

	// 9. Display preview
	fmt.Fprintln(out, "\nThe following packages will be installed:")
	for _, pkg := range toInstall {
		node := graph.GetNode(pkg)
		source := ""
		if node != nil {
			source = node.Source
		}
		fmt.Fprintf(out, "  • %-20s (%s)\n", pkg, source)
	}

	cliNames := make([]string, len(adapters))
	for i, a := range adapters {
		cliNames[i] = a.Name()
	}
	fmt.Fprintf(out, "\nInstalling on: %s\n", strings.Join(cliNames, ", "))

	// 10. Prompt for confirmation (unless --yes)
	if !installYes {
		fmt.Fprintf(out, "Proceed? [Y/n]: ")
		if !confirmPromptDefault(deps.stdin, true) {
			return fmt.Errorf("installation cancelled")
		}
	}

	// 11. Install all packages
	fmt.Fprintln(out)
	var installedDuringRun []string
	for _, pkg := range toInstall {
		node := graph.GetNode(pkg)
		source := specifier
		if node != nil && node.Source != "" {
			source = node.Source
		}

		var cliResults []string
		var installErr error
		for _, a := range adapters {
			if err := a.Install(source, scope); err != nil {
				installErr = fmt.Errorf("failed to install %s on %s: %w", pkg, a.Name(), err)
				break
			}
			cliResults = append(cliResults, a.Name())
		}

		if installErr != nil {
			if len(installedDuringRun) > 0 {
				fmt.Fprintf(deps.stderr, "\nPartial failure. Already installed during this run: %s\n",
					strings.Join(installedDuringRun, ", "))
			}
			return installErr
		}

		fmt.Fprintf(out, "  ✓ %s installed (%s)\n", pkg, strings.Join(cliResults, ", "))
		installedDuringRun = append(installedDuringRun, pkg)
	}

	fmt.Fprintf(out, "\nInstalled %d packages\n", len(installedDuringRun))
	return nil
}

func resolveTransitiveDeps(graph *depgraph.Graph, parentName string, m *manifest.Manifest, fetcher manifest.ManifestFetcher) error {
	for _, dep := range m.Dependencies {
		resolved, err := resolver.Resolve(dep)
		if err != nil {
			return fmt.Errorf("failed to resolve dependency %q: %w", dep, err)
		}

		depSource := dep
		if resolved.Source != "" {
			depSource = resolved.Source
		}

		if !graph.HasNode(resolved.Name) {
			graph.AddNode(&depgraph.Package{
				Name:   resolved.Name,
				Source: depSource,
			})

			// Recursively fetch manifest for this dependency
			depManifest, err := fetcher.FetchManifest(depSource)
			if err != nil {
				return fmt.Errorf("failed to fetch manifest for %s: %w", resolved.Name, err)
			}
			if depManifest != nil {
				if err := resolveTransitiveDeps(graph, resolved.Name, depManifest, fetcher); err != nil {
					return err
				}
			}
		}
		graph.AddEdge(parentName, resolved.Name)
	}
	return nil
}

func confirmPrompt(reader io.Reader) bool {
	scanner := bufio.NewScanner(reader)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

func confirmPromptDefault(reader io.Reader, defaultYes bool) bool {
	scanner := bufio.NewScanner(reader)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "" {
			return defaultYes
		}
		return answer == "y" || answer == "yes"
	}
	return defaultYes
}

// --- Command runners for production ---

type execRunner struct{}

func (r *execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r *execRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type execGitRunner struct{}

func (r *execGitRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
