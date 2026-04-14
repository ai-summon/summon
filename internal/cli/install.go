package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

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

// InstallResult tracks the outcome of installing a single package across CLIs.
type InstallResult struct {
	PackageName  string
	CLIResults   map[string]error
	Dependencies []string
}

// InstallSummary aggregates results across all packages in a single install invocation.
type InstallSummary struct {
	Results        []InstallResult
	TotalInstalled int
	TotalFailed    int
	CLIs           []string
}

// addResult records an install result and updates counters.
func (s *InstallSummary) addResult(r InstallResult) {
	s.Results = append(s.Results, r)
	failed := false
	for _, err := range r.CLIResults {
		if err != nil {
			failed = true
			break
		}
	}
	if failed {
		s.TotalFailed++
	} else {
		s.TotalInstalled++
	}
}

// installDeps holds the injectable dependencies for the install command.
type installDeps struct {
	runner   platform.CommandRunner
	fetcher  manifest.ManifestFetcher
	adapters []platform.Adapter // if non-nil, use instead of auto-detecting
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
}

// defaultInstallDeps returns the production dependencies.
func defaultInstallDeps() *installDeps {
	return &installDeps{
		runner:  &execRunner{},
		fetcher: manifest.NewLocalManifestFetcher(),
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
	var adapters []platform.Adapter
	if deps.adapters != nil {
		adapters = deps.adapters
	} else {
		allAdapters := []platform.Adapter{
			platform.NewCopilotAdapter(deps.runner),
			platform.NewClaudeAdapter(deps.runner),
		}
		for _, a := range allAdapters {
			if a.Detect() {
				adapters = append(adapters, a)
			} else {
				fmt.Fprintf(deps.stderr, "⚠ %s not detected, skipping\n", a.Name())
			}
		}
	}
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

	installSource, err := resolveInstallSource(resolved)
	if err != nil {
		return fmt.Errorf("failed to resolve install source: %w", err)
	}

	// 4. Ensure marketplace is registered (for marketplace-based installs)
	if resolved.Type == resolver.SourceOfficialMarketplace || resolved.Type == resolver.SourceNamedMarketplace {
		marketplaceName := "summon-marketplace"
		marketplaceSource := "ai-summon/summon-marketplace"
		if resolved.Type == resolver.SourceNamedMarketplace {
			marketplaceName = resolved.MarketplaceName
			marketplaceSource = resolved.MarketplaceName // named marketplaces use name as source
		}
		for _, a := range adapters {
			if ensureErr := a.EnsureMarketplace(marketplaceName, marketplaceSource); ensureErr != nil {
				fmt.Fprintf(deps.stderr, "⚠ %s: failed to ensure marketplace %q: %v\n", a.Name(), marketplaceName, ensureErr)
			}
		}
	}

	// 5. Initialize install summary
	cliNames := make([]string, len(adapters))
	for i, a := range adapters {
		cliNames[i] = a.Name()
	}
	summary := &InstallSummary{CLIs: cliNames}
	visited := make(map[string]bool)

	// 5. Install the package on each adapter
	result := InstallResult{
		PackageName: resolved.Name,
		CLIResults:  make(map[string]error),
	}
	for _, a := range adapters {
		fmt.Fprintf(out, "Installing %s on %s...\n", resolved.Name, a.Name())
		if installErr := a.Install(installSource, scope); installErr != nil {
			result.CLIResults[a.Name()] = fmt.Errorf("%s install %s failed: %w", a.Name(), installSource, installErr)
			fmt.Fprintf(out, "  ✗ %s failed on %s\n", resolved.Name, a.Name())
		} else {
			result.CLIResults[a.Name()] = nil
			fmt.Fprintf(out, "  ✓ %s installed on %s\n", resolved.Name, a.Name())
		}
	}
	visited[resolved.Name] = true

	// 6. Find local plugin dir and read manifest for transitive deps
	var m *manifest.Manifest
	for _, a := range adapters {
		if result.CLIResults[a.Name()] != nil {
			continue // skip failed CLIs
		}
		pluginDir, err := a.FindPluginDir(resolved.Name, scope)
		if err != nil {
			continue
		}
		m, err = deps.fetcher.FetchManifest(pluginDir)
		if err != nil {
			fmt.Fprintf(deps.stderr, "Warning: failed to read manifest for %s: %v\n", resolved.Name, err)
		}
		if m != nil {
			result.Dependencies = m.Dependencies
			break
		}
	}
	summary.addResult(result)

	// 7. Resolve transitive dependencies (if any)
	if m != nil && len(m.Dependencies) > 0 {
		if err := resolveTransitiveDeps(adapters, scope, m, deps.fetcher, visited, summary, out, deps.stderr); err != nil {
			// Continue — partial install is OK. Error is already reported in summary.
			fmt.Fprintf(deps.stderr, "Warning: transitive dependency resolution incomplete: %v\n", err)
		}
	}

	// 8. Check system requirements (unless --force)
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

		checkResult := syscheck.Check(reqs, nil)
		for _, r := range checkResult.Requirements {
			fmt.Fprintln(out, syscheck.FormatCheck(r))
		}

		if checkResult.HasRequired {
			fmt.Fprintf(deps.stderr, "\n⚠️  Required system dependencies are missing.\n")
		}
	}

	// 9. Display post-install summary
	renderSummary(summary, out)
	return nil
}

func resolveTransitiveDeps(adapters []platform.Adapter, scope platform.Scope, m *manifest.Manifest, fetcher manifest.ManifestFetcher, visited map[string]bool, summary *InstallSummary, out io.Writer, stderr io.Writer) error {
	for _, dep := range m.Dependencies {
		resolved, err := resolver.Resolve(dep)
		if err != nil {
			return fmt.Errorf("failed to resolve dependency %q: %w", dep, err)
		}

		// Cycle detection
		if visited[resolved.Name] {
			return fmt.Errorf("dependency cycle detected: %s already visited", resolved.Name)
		}
		visited[resolved.Name] = true

		depSource, err := resolveInstallSource(resolved)
		if err != nil {
			return fmt.Errorf("failed to resolve install source for dependency %q: %w", dep, err)
		}

		// Ensure marketplace for marketplace-based deps
		if resolved.Type == resolver.SourceOfficialMarketplace || resolved.Type == resolver.SourceNamedMarketplace {
			marketplaceName := "summon-marketplace"
			marketplaceSource := "ai-summon/summon-marketplace"
			if resolved.Type == resolver.SourceNamedMarketplace {
				marketplaceName = resolved.MarketplaceName
				marketplaceSource = resolved.MarketplaceName
			}
			for _, a := range adapters {
				if ensureErr := a.EnsureMarketplace(marketplaceName, marketplaceSource); ensureErr != nil {
					fmt.Fprintf(stderr, "⚠ %s: failed to ensure marketplace %q: %v\n", a.Name(), marketplaceName, ensureErr)
				}
			}
		}

		// Install dependency on each adapter
		result := InstallResult{
			PackageName: resolved.Name,
			CLIResults:  make(map[string]error),
		}
		for _, a := range adapters {
			fmt.Fprintf(out, "Installing dependency %s on %s...\n", resolved.Name, a.Name())
			if installErr := a.Install(depSource, scope); installErr != nil {
				result.CLIResults[a.Name()] = fmt.Errorf("%s install %s failed: %w", a.Name(), depSource, installErr)
				fmt.Fprintf(out, "  ✗ %s failed on %s\n", resolved.Name, a.Name())
			} else {
				result.CLIResults[a.Name()] = nil
				fmt.Fprintf(out, "  ✓ %s installed on %s\n", resolved.Name, a.Name())
			}
		}

		// Find plugin dir and read manifest for further transitive deps
		var depManifest *manifest.Manifest
		for _, a := range adapters {
			if result.CLIResults[a.Name()] != nil {
				continue
			}
			pluginDir, err := a.FindPluginDir(resolved.Name, scope)
			if err != nil {
				continue
			}
			depManifest, err = fetcher.FetchManifest(pluginDir)
			if err != nil {
				fmt.Fprintf(stderr, "Warning: failed to read manifest for %s: %v\n", resolved.Name, err)
			}
			if depManifest != nil {
				result.Dependencies = depManifest.Dependencies
				break
			}
		}
		summary.addResult(result)

		// Recurse for transitive deps
		if depManifest != nil && len(depManifest.Dependencies) > 0 {
			if err := resolveTransitiveDeps(adapters, scope, depManifest, fetcher, visited, summary, out, stderr); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveInstallSource resolves a ResolvedSource to an installable source string.
// For marketplace types, formats as name@marketplace for native CLI delegation.
func resolveInstallSource(resolved *resolver.ResolvedSource) (string, error) {
	switch resolved.Type {
	case resolver.SourceOfficialMarketplace:
		return resolved.Name + "@summon-marketplace", nil
	case resolver.SourceNamedMarketplace:
		return resolved.Name + "@" + resolved.MarketplaceName, nil
	default:
		// SourceGitHubShorthand, SourceDirectURL, SourceNativeMarketplace
		if resolved.Source != "" {
			return resolved.Source, nil
		}
		return resolved.Name, nil
	}
}

// renderSummary prints a post-install summary.
func renderSummary(summary *InstallSummary, out io.Writer) {
	total := summary.TotalInstalled + summary.TotalFailed
	if total == 0 {
		return
	}

	if summary.TotalFailed == 0 {
		fmt.Fprintf(out, "\nInstalled %d packages:\n", total)
	} else {
		fmt.Fprintf(out, "\nInstalled %d of %d packages:\n", summary.TotalInstalled, total)
	}

	for _, r := range summary.Results {
		var successCLIs []string
		var failedParts []string
		for _, cli := range summary.CLIs {
			if err, ok := r.CLIResults[cli]; ok {
				if err != nil {
					failedParts = append(failedParts, fmt.Sprintf("%s: failed — %q", cli, err.Error()))
				} else {
					successCLIs = append(successCLIs, cli)
				}
			}
		}

		if len(failedParts) == 0 {
			fmt.Fprintf(out, "  ✓ %-20s (%s)\n", r.PackageName, strings.Join(successCLIs, ", "))
		} else if len(successCLIs) > 0 {
			fmt.Fprintf(out, "  ✓ %-20s (%s)\n", r.PackageName, strings.Join(successCLIs, ", "))
			for _, fp := range failedParts {
				fmt.Fprintf(out, "  ✗ %-20s (%s)\n", r.PackageName, fp)
			}
		} else {
			for _, fp := range failedParts {
				fmt.Fprintf(out, "  ✗ %-20s (%s)\n", r.PackageName, fp)
			}
		}
	}
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
