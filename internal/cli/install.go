package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

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
	noColor  bool
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
		deps.noColor = noColorFlag
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
	s := NewStyles(deps.noColor)

	// 1. Parse the scope
	scope, err := platform.ParseScope(installScope)
	if err != nil {
		return err
	}

	// 2. Detect platform adapters
	adapters, err := resolveEnabledAdapters(&adapterResolverDeps{
		runner:   deps.runner,
		adapters: deps.adapters,
		target:   targetFlag,
		stderr:   deps.stderr,
	})
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
				fmt.Fprintf(deps.stderr, "%s %s: failed to ensure marketplace %q: %v\n", s.StatusIcon("warn"), a.Name(), marketplaceName, ensureErr)
			}
		}
	}

	// 5. Initialize tracking
	cliNames := make([]string, len(adapters))
	for i, a := range adapters {
		cliNames[i] = a.Name()
	}
	manifestCache := make(map[string]*manifest.Manifest)
	resultMap := make(map[string]*InstallResult)
	var resultOrder []string

	// 6. Platform-first install loop
	for _, a := range adapters {
		fmt.Fprintln(out, s.PlatformHeader(a.Name()))

		// Install main package
		fmt.Fprintf(out, "  Installing %s...", resolved.Name)
		if installErr := a.Install(installSource, scope); installErr != nil {
			ensureResult(resultMap, &resultOrder, resolved.Name)
			resultMap[resolved.Name].CLIResults[a.Name()] = fmt.Errorf("%s install %s failed: %w", a.Name(), installSource, installErr)
			fmt.Fprintf(out, " %s %s failed\n", s.StatusIcon("fail"), resolved.Name)
			continue
		}
		ensureResult(resultMap, &resultOrder, resolved.Name)
		resultMap[resolved.Name].CLIResults[a.Name()] = nil
		fmt.Fprintf(out, " %s %s installed\n", s.StatusIcon("pass"), resolved.Name)

		// Read manifest (use cache or read from disk)
		m := getOrCacheManifest(resolved.Name, manifestCache, a, scope, deps.fetcher, deps.stderr)

		// Resolve transitive deps for this single adapter
		if m != nil && len(m.Dependencies) > 0 {
			visited := map[string]bool{resolved.Name: true}
			if depErr := resolveTransitiveDeps(a, scope, m, deps.fetcher, visited, resultMap, &resultOrder, manifestCache, "    ", out, deps.stderr, s); depErr != nil {
				fmt.Fprintf(deps.stderr, "Warning: %s: transitive dependency resolution incomplete: %v\n", a.Name(), depErr)
			}
		}
	}

	// Populate main package dependencies from cached manifest
	if m, ok := manifestCache[resolved.Name]; ok && m != nil {
		if r, exists := resultMap[resolved.Name]; exists {
			r.Dependencies = m.Dependencies
		}
	}

	// 7. Build summary from accumulated results
	summary := &InstallSummary{CLIs: cliNames}
	for _, name := range resultOrder {
		summary.addResult(*resultMap[name])
	}

	// 8. Check system requirements (unless --force)
	if m, ok := manifestCache[resolved.Name]; ok && m != nil && len(m.SystemRequirements) > 0 && !installForce {
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
			fmt.Fprintf(deps.stderr, "\n%s Required system dependencies are missing.\n", s.StatusIcon("warn"))
		}
	}

	// 9. Display post-install summary
	renderSummary(summary, out, s)
	return nil
}

func resolveTransitiveDeps(adapter platform.Adapter, scope platform.Scope, m *manifest.Manifest, fetcher manifest.ManifestFetcher, visited map[string]bool, resultMap map[string]*InstallResult, resultOrder *[]string, manifestCache map[string]*manifest.Manifest, indent string, out io.Writer, stderr io.Writer, s Styles) error {
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
			if ensureErr := adapter.EnsureMarketplace(marketplaceName, marketplaceSource); ensureErr != nil {
				fmt.Fprintf(stderr, "%s %s: failed to ensure marketplace %q: %v\n", s.StatusIcon("warn"), adapter.Name(), marketplaceName, ensureErr)
			}
		}

		// Install dependency
		ensureResult(resultMap, resultOrder, resolved.Name)
		fmt.Fprintf(out, "%sInstalling dependency %s...", indent, resolved.Name)
		if installErr := adapter.Install(depSource, scope); installErr != nil {
			resultMap[resolved.Name].CLIResults[adapter.Name()] = fmt.Errorf("%s install %s failed: %w", adapter.Name(), depSource, installErr)
			fmt.Fprintf(out, " %s %s failed\n", s.StatusIcon("fail"), resolved.Name)
			continue
		}
		resultMap[resolved.Name].CLIResults[adapter.Name()] = nil
		fmt.Fprintf(out, " %s %s installed\n", s.StatusIcon("pass"), resolved.Name)

		// Read manifest for further transitive deps
		depManifest := getOrCacheManifest(resolved.Name, manifestCache, adapter, scope, fetcher, stderr)
		if depManifest != nil {
			resultMap[resolved.Name].Dependencies = depManifest.Dependencies
		}

		// Recurse for transitive deps
		if depManifest != nil && len(depManifest.Dependencies) > 0 {
			if err := resolveTransitiveDeps(adapter, scope, depManifest, fetcher, visited, resultMap, resultOrder, manifestCache, indent+"  ", out, stderr, s); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureResult creates an InstallResult in the result map if it doesn't exist yet,
// and appends the package name to the result order for deterministic summary output.
func ensureResult(resultMap map[string]*InstallResult, resultOrder *[]string, name string) {
	if _, exists := resultMap[name]; !exists {
		resultMap[name] = &InstallResult{PackageName: name, CLIResults: make(map[string]error)}
		*resultOrder = append(*resultOrder, name)
	}
}

// getOrCacheManifest reads a manifest from cache or from disk via the adapter's plugin directory.
// Only non-nil manifests are cached so that subsequent adapters can retry on failure.
func getOrCacheManifest(name string, cache map[string]*manifest.Manifest, a platform.Adapter, scope platform.Scope, fetcher manifest.ManifestFetcher, stderr io.Writer) *manifest.Manifest {
	if m, ok := cache[name]; ok {
		return m
	}
	pluginDir, err := a.FindPluginDir(name, scope)
	if err != nil {
		return nil
	}
	m, err := fetcher.FetchManifest(pluginDir)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: failed to read manifest for %s: %v\n", name, err)
		return nil
	}
	if m != nil {
		cache[name] = m
	}
	return m
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
func renderSummary(summary *InstallSummary, out io.Writer, s Styles) {
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
			fmt.Fprintf(out, "  %s %-20s (%s)\n", s.StatusIcon("pass"), r.PackageName, strings.Join(successCLIs, ", "))
		} else if len(successCLIs) > 0 {
			fmt.Fprintf(out, "  %s %-20s (%s)\n", s.StatusIcon("pass"), r.PackageName, strings.Join(successCLIs, ", "))
			for _, fp := range failedParts {
				fmt.Fprintf(out, "  %s %-20s (%s)\n", s.StatusIcon("fail"), r.PackageName, fp)
			}
		} else {
			for _, fp := range failedParts {
				fmt.Fprintf(out, "  %s %-20s (%s)\n", s.StatusIcon("fail"), r.PackageName, fp)
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

// defaultRunTimeout is the maximum time a single CLI command is allowed to run.
const defaultRunTimeout = 30 * time.Second

type execRunner struct{}

func (r *execRunner) Run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRunTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("timed out after %s", defaultRunTimeout)
	}
	return out, err
}

func (r *execRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type execGitRunner struct{}

func (r *execGitRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
