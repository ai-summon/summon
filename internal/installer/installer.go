// Package installer handles the installation, restoration, and management of
// summon packages. It supports installing packages from GitHub repositories,
// local filesystem paths, and the summon marketplace. Installed packages are
// tracked in a registry and integrated with supported AI platforms via their
// CLIs — summon never reads or writes any platform configuration file.
package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-summon/summon/internal/git"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/resolver"
	"github.com/ai-summon/summon/internal/store"
	"golang.org/x/term"
)

// Options configures a single package installation. Package is a git URL,
// marketplace name, or GitHub shorthand; Path is set for local installs instead.
// Ref pins a specific git tag/branch, Force bypasses compatibility checks.
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
// registry file, and Scope indicates the installation scope.
type Paths struct {
	StoreDir     string
	RegistryPath string
	Scope        platform.Scope
}

// ResolvePaths computes the store and registry directories for one writable scope.
func ResolvePaths(scope platform.Scope, projectDir string) Paths {
	if scope == platform.ScopeUser {
		home, _ := os.UserHomeDir()
		base := filepath.Join(home, ".summon", "user")
		return Paths{
			StoreDir:     filepath.Join(base, "store"),
			RegistryPath: filepath.Join(base, "registry.yaml"),
			Scope:        platform.ScopeUser,
		}
	}

	base := filepath.Join(projectDir, ".summon", scope.String())
	return Paths{
		StoreDir:     filepath.Join(base, "store"),
		RegistryPath: filepath.Join(base, "registry.yaml"),
		Scope:        scope,
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

// MakeScopedTempDir creates a temporary staging directory near the selected
// scope store so same-filesystem renames remain the fast-path.
func MakeScopedTempDir(paths Paths, pattern string) (string, error) {
	base := filepath.Dir(paths.StoreDir)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("creating staging base dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(base, pattern)
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	return tmpDir, nil
}

// Install installs a single package according to opts. It delegates to
// installLocal when opts.Path is set, installMarketplace for bare names,
// or installGitHub for github: prefixed or URL packages.
func Install(opts Options) error {
	paths := ResolvePaths(scopeFromLegacy(opts.Global, opts.Scope), opts.ProjectDir)
	s := store.New(paths.StoreDir)
	reg, err := registry.Load(paths.RegistryPath)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}
	reg.Scope = paths.Scope.String()

	// Detect platforms and enforce prerequisites.
	activePlatforms := platform.DetectActive(opts.ProjectDir)
	if len(activePlatforms) == 0 {
		return fmt.Errorf("No supported AI platform detected.\n\n" +
			"Summon requires at least one of the following:\n" +
			"  • Claude Code (https://code.claude.com)\n" +
			"  • GitHub Copilot CLI (https://github.com/features/copilot)\n\n" +
			"Install one of the above platforms and try again.")
	}

	// Scope validation: skip platforms that don't support the requested scope.
	var compatiblePlatforms []platform.Adapter
	var skippedPlatforms []string
	for _, a := range activePlatforms {
		if platform.SupportsScope(a, paths.Scope) {
			compatiblePlatforms = append(compatiblePlatforms, a)
		} else {
			skippedPlatforms = append(skippedPlatforms, a.Name())
			StatusErr("Info", "%s does not support %s scope. Skipping %s.", a.Name(), paths.Scope, a.Name())
		}
	}
	if len(compatiblePlatforms) == 0 {
		return fmt.Errorf("No platform supports %s scope.\n\n"+
			"GitHub Copilot only supports user scope.\n"+
			"To install for Copilot, use:\n"+
			"  summon install <package> --scope user", paths.Scope)
	}

	if opts.Path != "" {
		return installLocal(opts, paths, s, reg, compatiblePlatforms)
	}
	return installGitHub(opts, paths, s, reg, compatiblePlatforms)
}

func installGitHub(opts Options, paths Paths, s *store.Store, reg *registry.Registry, platforms []platform.Adapter) error {
	gitURL, err := resolveGitURL(opts.Package)
	if err != nil {
		return err
	}
	name := packageNameFromURL(gitURL)

	if entry, ok := reg.Get(name); ok && opts.Ref == "" && !opts.Force {
		Status("Up-to-date", "%s is already installed at version %s", name, entry.Version)
		return nil
	}

	tmpDir, err := MakeScopedTempDir(paths, "summon-install-*")
	if err != nil {
		return err
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

	if err := s.MoveFromStage(name, cloneDest); err != nil {
		return fmt.Errorf("moving to store: %w", err)
	}
	storePath := s.PackagePath(name)

	if err := expandHookVariables(storePath); err != nil {
		return fmt.Errorf("expanding hook variables: %w", err)
	}

	version := ref
	if version == "HEAD" {
		version = sha[:8]
	}

	platformNames := getPlatformNames(platforms)
	reg.Add(name, registry.Entry{
		Version: version,
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

	discoverOnPlatforms(name, storePath, paths.Scope, platforms)

	Status("Installed", "%s %s (%s) → %s scope", name, version, sha[:8], paths.Scope.String())
	return nil
}

func installLocal(opts Options, paths Paths, s *store.Store, reg *registry.Registry, platforms []platform.Adapter) error {
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	name := filepath.Base(absPath)
	if err := s.Link(name, absPath); err != nil {
		return fmt.Errorf("linking to store: %w", err)
	}

	storePath := s.PackagePath(name)
	if err := expandHookVariables(storePath); err != nil {
		return fmt.Errorf("expanding hook variables: %w", err)
	}

	platformNames := getPlatformNames(platforms)
	reg.Add(name, registry.Entry{
		Version: "local",
		Source: registry.Source{
			Type: "local",
			URL:  absPath,
		},
		Platforms: platformNames,
	})
	if err := reg.Save(paths.RegistryPath); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}

	discoverOnPlatforms(name, storePath, paths.Scope, platforms)

	Status("Installed", "%s (local) → %s scope", name, paths.Scope.String())
	return nil
}

// discoverOnPlatforms calls DiscoverPackage on each platform adapter.
// Errors are logged as warnings but do not fail the install.
func discoverOnPlatforms(name, pkgPath string, scope platform.Scope, platforms []platform.Adapter) {
	for _, a := range platforms {
		if err := a.DiscoverPackage(name, pkgPath, scope); err != nil {
			StatusErr("Warning", "failed to register with %s: %v", a.Name(), err)
		}
	}
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
	activePlatforms := platform.DetectActive(projectDir)

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
		_ = expandHookVariables(storePath)

		// Re-register with platforms
		for _, a := range activePlatforms {
			if platform.SupportsScope(a, scope) {
				_ = a.DiscoverPackage(name, storePath, scope)
			}
		}
	}

	Status("Restored", "all packages in %s scope", scope.String())
	return nil
}

// resolveGitURL converts a package specifier into a cloneable git URL.
// It recognises "github:user/repo" shorthand and full https:// or git@ URLs.
// Bare names are not yet supported (marketplace resolution is planned).
func resolveGitURL(pkg string) (string, error) {
	if strings.HasPrefix(pkg, "github:") {
		path := strings.TrimPrefix(pkg, "github:")
		return "https://github.com/" + path, nil
	}
	if strings.HasPrefix(pkg, "https://") || strings.HasPrefix(pkg, "git@") || strings.HasPrefix(pkg, "file://") {
		return pkg, nil
	}
	// Bare name: assume GitHub org ai-summon (marketplace convention)
	return "https://github.com/ai-summon/" + pkg, nil
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

// getPlatformNames extracts the Name() string from each adapter.
func getPlatformNames(adapters []platform.Adapter) []string {
	var names []string
	for _, a := range adapters {
		names = append(names, a.Name())
	}
	return names
}

// EnsureGitignore appends scope-specific .summon/ entries to
// <projectDir>/.gitignore, creating the file if it does not exist.
// The local scope directory is fully gitignored; only project/store/ is
// gitignored (project/registry.yaml may be committed).
// Entries that already appear in the file are not duplicated.
func EnsureGitignore(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	entries := []string{
		".summon/local/",
		".summon/project/store/",
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

// Stdin is the reader used for interactive prompts. Tests can replace this.
var Stdin *os.File = os.Stdin

// IsInteractive checks whether stdin is a terminal and SUMMON_NONINTERACTIVE
// is not set.
func IsInteractive() bool {
	if os.Getenv("SUMMON_NONINTERACTIVE") == "1" {
		return false
	}
	return term.IsTerminal(int(Stdin.Fd()))
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
