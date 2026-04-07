// Package installer handles the installation, restoration, and management of
// summon packages. It supports installing packages from GitHub repositories,
// local filesystem paths, and the built-in catalog. Installed packages are
// tracked in a registry and integrated with supported AI platforms (e.g.
// Claude Code, VS Code Copilot).
package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-summon/summon/internal/catalog"
	"github.com/ai-summon/summon/internal/git"
	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/marketplace"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/resolver"
	"github.com/ai-summon/summon/internal/store"
)

// Options configures a single package installation. Package is a git URL,
// catalog name, or GitHub shorthand; Path is set for local installs instead.
// Ref pins a specific git tag/branch, Force bypasses compatibility and
// duplicate-version checks, and Global selects user-wide vs project-local scope.
type Options struct {
	Package       string
	Path          string
	Ref           string
	Force         bool
	Global        bool
	Scope         platform.Scope
	ProjectDir    string
	SummonVersion string
}

// Paths holds the resolved filesystem locations used during installation.
// StoreDir is where package contents live, RegistryPath points to the YAML
// registry file, PlatformsDir holds per-platform marketplace data, Scope
// indicates global vs local, and MarketplaceName is the marketplace identifier
// registered with each platform.
type Paths struct {
	StoreDir        string
	RegistryPath    string
	PlatformsDir    string
	Scope           platform.Scope
	MarketplaceName string
}

// ResolvePaths computes the store, registry, and platform directories for one
// writable scope.
func ResolvePaths(scope platform.Scope, projectDir string) Paths {
	if scope == platform.ScopeUser {
		home, _ := os.UserHomeDir()
		base := filepath.Join(home, ".summon", "user")
		return Paths{
			StoreDir:        filepath.Join(base, "store"),
			RegistryPath:    filepath.Join(base, "registry.yaml"),
			PlatformsDir:    filepath.Join(base, "platforms"),
			Scope:           platform.ScopeUser,
			MarketplaceName: "summon-user",
		}
	}

	base := filepath.Join(projectDir, ".summon", scope.String())
	marketplaceName := "summon-" + scope.String()
	return Paths{
		StoreDir:        filepath.Join(base, "store"),
		RegistryPath:    filepath.Join(base, "registry.yaml"),
		PlatformsDir:    filepath.Join(base, "platforms"),
		Scope:           scope,
		MarketplaceName: marketplaceName,
	}
}

func scopeFromLegacy(global bool, scope platform.Scope) platform.Scope {
	if scope == platform.ScopeLocal || scope == platform.ScopeProject || scope == platform.ScopeUser {
		return scope
	}
	if global {
		return platform.ScopeUser
	}
	return platform.ScopeLocal
}

// Install installs a single package according to opts. It delegates to
// installLocal when opts.Path is set, or installGitHub otherwise.
func Install(opts Options) error {
	paths := ResolvePaths(scopeFromLegacy(opts.Global, opts.Scope), opts.ProjectDir)
	s := store.New(paths.StoreDir)
	reg, err := registry.Load(paths.RegistryPath)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	reg.Scope = paths.Scope.String()
	if opts.Path != "" {
		return installLocal(opts, paths, s, reg)
	}
	return installGitHub(opts, paths, s, reg)
}

func installGitHub(opts Options, paths Paths, s *store.Store, reg *registry.Registry) error {
	gitURL, err := resolveGitURL(opts.Package)
	if err != nil {
		return err
	}
	name := packageNameFromURL(gitURL)

	if entry, ok := reg.Get(name); ok && opts.Ref == "" && !opts.Force {
		Status("Up-to-date", "%s is already installed at version %s", name, entry.Version)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "summon-install-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, name)
	Status("Fetching", "%s", gitURL)
	if err := git.Clone(gitURL, cloneDest); err != nil {
		return fmt.Errorf("cloning %s: %w", gitURL, err)
	}

	ref := opts.Ref
	if ref == "" {
		ref, err = resolver.ResolveLatest(cloneDest)
		if err != nil {
			return fmt.Errorf("resolving version: %w", err)
		}
	}
	if ref != "HEAD" {
		if err := git.Checkout(cloneDest, ref); err != nil {
			return fmt.Errorf("checking out %s: %w", ref, err)
		}
	}

	sha, err := git.RevParseHEAD(cloneDest)
	if err != nil {
		return fmt.Errorf("getting commit SHA: %w", err)
	}
	if entry, ok := reg.Get(name); ok && entry.Source.SHA == sha && !opts.Force {
		Status("Up-to-date", "%s is already at %s (%s)", name, ref, sha[:8])
		return nil
	}

	manifests, pluginRoots, err := manifest.LoadOrInfer(cloneDest)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	for i, m := range manifests {
		pluginRoot := pluginRoots[i]
		hasSummonYAML := fileExists(filepath.Join(pluginRoot, "summon.yaml"))

		if hasSummonYAML {
			if errs := m.ValidateFull(pluginRoot); len(errs) > 0 {
				for _, e := range errs {
					StatusErr("Warning", "%s", e)
				}
			}
		}

		if opts.SummonVersion != "" {
			if ok, msg := manifest.CheckSummonVersion(m.SummonVersion, opts.SummonVersion); !ok {
				return fmt.Errorf("version constraint: %s", msg)
			}
		}

		activePlatforms := platform.DetectActive(opts.ProjectDir)
		compatiblePlatforms := filterCompatible(m.Platforms, activePlatforms)
		if len(compatiblePlatforms) == 0 && !opts.Force {
			return fmt.Errorf("no compatible platform detected for %s (supports: %v). Use --force to install anyway", m.Name, m.Platforms)
		}
		if len(compatiblePlatforms) == 0 {
			StatusErr("Warning", "no compatible platform detected, installing with --force")
			if len(m.Platforms) > 0 {
				compatiblePlatforms = makePlatformList(m.Platforms, opts.ProjectDir)
			}
		}

		if err := s.Remove(m.Name); err != nil {
			return fmt.Errorf("removing old store entry: %w", err)
		}
		storePath := s.PackagePath(m.Name)
		if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
			return err
		}
		if err := os.Rename(pluginRoot, storePath); err != nil {
			return fmt.Errorf("moving to store: %w", err)
		}

		if !marketplace.PluginJSONExists(storePath) {
			if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
				return fmt.Errorf("generating plugin.json: %w", err)
			}
		}
		if err := expandHookVariables(storePath); err != nil {
			return fmt.Errorf("expanding hook variables: %w", err)
		}

		platformNames := getPlatformNames(compatiblePlatforms)
		reg.Add(m.Name, registry.Entry{
			Version: m.Version,
			Source: registry.Source{
				Type: "github",
				URL:  gitURL,
				Ref:  ref,
				SHA:  sha,
			},
			Platforms: platformNames,
		})
		if err := reg.Save(paths.RegistryPath); err != nil {
			return fmt.Errorf("saving registry: %w", err)
		}

		if err := generateMarketplaces(paths, reg); err != nil {
			return fmt.Errorf("generating marketplace: %w", err)
		}
		registerPlatforms(paths, compatiblePlatforms)
		enablePlugins(m.Name, paths, compatiblePlatforms)
		materializeComponents(storePath, m, paths, compatiblePlatforms)

		Status("Installed", "%s v%s (%s) → %s scope", m.Name, m.Version, sha[:8], paths.Scope.String())
		reportDependencies(m, reg)
	}
	return nil
}

func installLocal(opts Options, paths Paths, s *store.Store, reg *registry.Registry) error {
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	manifests, pluginRoots, err := manifest.LoadOrInfer(absPath)
	if err != nil {
		return err
	}

	for i, m := range manifests {
		pluginRoot := pluginRoots[i]
		hasSummonYAML := fileExists(filepath.Join(pluginRoot, "summon.yaml"))

		if hasSummonYAML {
			if errs := m.ValidateFull(pluginRoot); len(errs) > 0 {
				for _, e := range errs {
					StatusErr("Warning", "%s", e)
				}
			}
		}

		if opts.SummonVersion != "" {
			if ok, msg := manifest.CheckSummonVersion(m.SummonVersion, opts.SummonVersion); !ok {
				return fmt.Errorf("version constraint: %s", msg)
			}
		}

		activePlatforms := platform.DetectActive(opts.ProjectDir)
		compatiblePlatforms := filterCompatible(m.Platforms, activePlatforms)
		if len(compatiblePlatforms) == 0 && !opts.Force {
			return fmt.Errorf("no compatible platform detected for %s (supports: %v). Use --force to install anyway", m.Name, m.Platforms)
		}

		if err := s.Link(m.Name, pluginRoot); err != nil {
			return fmt.Errorf("linking to store: %w", err)
		}

		storePath := s.PackagePath(m.Name)
		if !marketplace.PluginJSONExists(storePath) {
			if err := marketplace.GeneratePluginJSON(storePath, m); err != nil {
				return fmt.Errorf("generating plugin.json: %w", err)
			}
		}
		if err := expandHookVariables(storePath); err != nil {
			return fmt.Errorf("expanding hook variables: %w", err)
		}

		platformNames := getPlatformNames(compatiblePlatforms)
		reg.Add(m.Name, registry.Entry{
			Version: m.Version,
			Source: registry.Source{
				Type: "local",
				URL:  pluginRoot,
			},
			Platforms: platformNames,
		})
		if err := reg.Save(paths.RegistryPath); err != nil {
			return fmt.Errorf("saving registry: %w", err)
		}

		if err := generateMarketplaces(paths, reg); err != nil {
			return fmt.Errorf("generating marketplace: %w", err)
		}
		registerPlatforms(paths, compatiblePlatforms)
		enablePlugins(m.Name, paths, compatiblePlatforms)
		materializeComponents(storePath, m, paths, compatiblePlatforms)

		Status("Installed", "%s v%s (local) → %s scope", m.Name, m.Version, paths.Scope.String())
		reportDependencies(m, reg)
	}
	return nil
}

// RestoreAll re-fetches every package recorded in the registry that is not
// already present in the store. This is used after cloning a project to
// recreate the .summon/store directory from the registry.yaml lock file.
func RestoreAll(global bool, projectDir string) error {
	return RestoreScope(scopeFromLegacy(global, 0), projectDir)
}

// RestoreScope rehydrates packages recorded for one writable scope.
func RestoreScope(scope platform.Scope, projectDir string) error {
	paths := ResolvePaths(scope, projectDir)
	reg, err := registry.Load(paths.RegistryPath)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	reg.Scope = paths.Scope.String()
	if len(reg.Packages) == 0 {
		fmt.Fprintln(Stdout, "No packages to restore.")
		return nil
	}

	s := store.New(paths.StoreDir)
	for name, entry := range reg.Packages {
		if s.Has(name) {
			Status("Up-to-date", "%s already in store, skipping", name)
			continue
		}
		switch entry.Source.Type {
		case "github":
			Status("Restoring", "%s from %s", name, entry.Source.URL)
			ref := entry.Source.Ref
			if ref == "" {
				ref = "HEAD"
			}
			if err := git.CloneRef(entry.Source.URL, s.PackagePath(name), ref); err != nil {
				StatusErr("Error", "failed to restore %s: %v", name, err)
				continue
			}
		case "local":
			Status("Restoring", "%s from %s", name, entry.Source.URL)
			if _, statErr := os.Stat(entry.Source.URL); statErr != nil {
				StatusErr("Warning", "local path %s not available for %s", entry.Source.URL, name)
				continue
			}
			if err := s.Link(name, entry.Source.URL); err != nil {
				StatusErr("Error", "failed to restore %s: %v", name, err)
				continue
			}
		}
		storePath := s.PackagePath(name)
		m, mErr := manifest.Load(storePath)
		if mErr == nil {
			if !marketplace.PluginJSONExists(storePath) {
				_ = marketplace.GeneratePluginJSON(storePath, m)
			}
			_ = expandHookVariables(storePath)
		}
	}

	if err := generateMarketplaces(paths, reg); err != nil {
		return fmt.Errorf("generating marketplaces: %w", err)
	}
	activePlatforms := platform.DetectActive(projectDir)
	registerPlatforms(paths, activePlatforms)

	Status("Restored", "all packages in %s scope", scope.String())
	return nil
}

// resolveGitURL converts a package specifier into a cloneable git URL.
// It recognises "github:user/repo" shorthand, full https:// or git@ URLs,
// and bare catalog names that are looked up via catalog.LoadDefault.
func resolveGitURL(pkg string) (string, error) {
	if strings.HasPrefix(pkg, "github:") {
		path := strings.TrimPrefix(pkg, "github:")
		return "https://github.com/" + path, nil
	}
	if strings.HasPrefix(pkg, "https://") || strings.HasPrefix(pkg, "git@") {
		return pkg, nil
	}
	cat, err := catalog.LoadDefault()
	if err != nil {
		return "", fmt.Errorf("loading catalog: %w", err)
	}
	entry, ok := cat.Lookup(pkg)
	if !ok {
		return "", fmt.Errorf("package %q not found in catalog. Use github:user/repo for direct GitHub URLs", pkg)
	}
	return entry.Repository, nil
}

// packageNameFromURL extracts a short package name from a git URL by taking
// the last path segment after stripping any ".git" suffix.
func packageNameFromURL(url string) string {
	parts := strings.Split(strings.TrimSuffix(url, ".git"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return url
}

// filterCompatible returns the subset of active platform adapters whose names
// appear in manifestPlatforms. If the manifest declares no platforms, all
// active adapters are considered compatible.
func filterCompatible(manifestPlatforms []string, active []platform.Adapter) []platform.Adapter {
	if len(manifestPlatforms) == 0 {
		return active
	}
	var compatible []platform.Adapter
	for _, a := range active {
		for _, p := range manifestPlatforms {
			if a.Name() == p {
				compatible = append(compatible, a)
				break
			}
		}
	}
	return compatible
}

// makePlatformList returns adapters for the named platforms regardless of
// whether they are currently detected. This is used with --force to install
// a package even when the target platform is not active.
func makePlatformList(platforms []string, projectDir string) []platform.Adapter {
	all := platform.AllAdapters(projectDir)
	var result []platform.Adapter
	for _, a := range all {
		for _, p := range platforms {
			if a.Name() == p {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// getPlatformNames extracts the Name() string from each adapter.
func getPlatformNames(adapters []platform.Adapter) []string {
	var names []string
	for _, a := range adapters {
		names = append(names, a.Name())
	}
	return names
}

// generateMarketplaces regenerates the marketplace index for every known
// platform so that each platform's plugin registry reflects the current
// set of installed packages.
func generateMarketplaces(paths Paths, reg *registry.Registry) error {
	for _, a := range platform.AllAdapters("") {
		platformDir := filepath.Join(paths.PlatformsDir, a.Name())
		if err := marketplace.Generate(a.Name(), paths.MarketplaceName, paths.StoreDir, platformDir, reg); err != nil {
			return err
		}
	}
	return nil
}

// registerPlatforms tells each compatible platform adapter about the
// marketplace directory so the platform can discover installed plugins.
// Errors from individual adapters are logged as warnings but do not fail
// the install.
func registerPlatforms(paths Paths, adapters []platform.Adapter) {
	for _, a := range adapters {
		platformDir := filepath.Join(paths.PlatformsDir, a.Name())
		if err := a.Register(platformDir, paths.MarketplaceName, paths.Scope); err != nil {
			StatusErr("Warning", "failed to register with %s: %v", a.Name(), err)
		}
	}
}

// enablePlugins auto-enables a plugin on each compatible platform so the user
// does not need an additional manual step after summon install. Errors from
// individual adapters are logged as warnings but do not fail the install.
func enablePlugins(pluginName string, paths Paths, adapters []platform.Adapter) {
	for _, a := range adapters {
		if err := a.EnablePlugin(pluginName, paths.MarketplaceName, paths.StoreDir, paths.Scope); err != nil {
			StatusErr("Warning", "failed to enable plugin on %s: %v", a.Name(), err)
		}
	}
}

// materializeComponents calls MaterializeComponents on any adapter that
// implements the Materializer interface, for project and local scopes where
// workspace-visible component paths can be created. Errors logged as warnings.
func materializeComponents(storePath string, m *manifest.Manifest, paths Paths, adapters []platform.Adapter) {
	if paths.Scope != platform.ScopeProject && paths.Scope != platform.ScopeLocal {
		return
	}
	for _, a := range adapters {
		mat, ok := a.(platform.Materializer)
		if !ok {
			continue
		}
		if err := mat.MaterializeComponents(storePath, m, paths.Scope); err != nil {
			StatusErr("Warning", "Copilot workspace materialization for %s: %v", m.Name, err)
		}
	}
}

// EnsureGitignore appends scope-specific .summon/ entries to
// <projectDir>/.gitignore, creating the file if it does not exist.
// The local scope directory is fully gitignored; only project/store/ and
// project/platforms/ are gitignored (project/registry.yaml may be committed).
// Entries that already appear in the file are not duplicated.
func EnsureGitignore(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	entries := []string{
		".summon/local/",
		".summon/project/store/",
		".summon/project/platforms/",
	}
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(existing)
	var toAdd []string
	for _, e := range entries {
		if !strings.Contains(content, e) {
			toAdd = append(toAdd, e)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	for _, e := range toAdd {
		if _, err := f.WriteString(e + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// reportDependencies prints a warning to stderr listing any manifest
// dependencies that are not yet present in the registry.
func reportDependencies(m *manifest.Manifest, reg *registry.Registry) {
	if len(m.Dependencies) == 0 {
		return
	}
	var missing []string
	for dep := range m.Dependencies {
		if !reg.Has(dep) {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		StatusErr("Warning", "missing dependencies:")
		for _, dep := range missing {
			fmt.Fprintf(Stderr, "%*s - %s (install with: summon install %s)\n", statusLabelWidth, "", dep, dep)
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// expandHookVariables replaces ${CLAUDE_PLUGIN_ROOT} in hooks.json with the
// absolute plugin path. This is needed because some platforms (VS Code Copilot
// Chat) don't expand this template variable before executing hooks.
//
// It checks both known hook file locations to match the detection logic in
// manifest.detectComponents:
//   - <storePath>/hooks/hooks.json  (subdirectory layout)
//   - <storePath>/hooks.json        (root-level layout)
func expandHookVariables(storePath string) error {
	absPath, err := filepath.Abs(storePath)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}

	candidates := []string{
		filepath.Join(storePath, "hooks", "hooks.json"),
		filepath.Join(storePath, "hooks.json"),
	}

	for _, hooksPath := range candidates {
		data, err := os.ReadFile(hooksPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		expanded := strings.ReplaceAll(string(data), "${CLAUDE_PLUGIN_ROOT}", absPath)
		if expanded == string(data) {
			continue
		}

		if err := os.WriteFile(hooksPath, []byte(expanded), 0o644); err != nil {
			return err
		}
	}

	return nil
}
