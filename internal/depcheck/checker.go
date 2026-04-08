package depcheck

import (
	"fmt"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
)

// InstalledPackage represents a package found in the registry with its scope.
type InstalledPackage struct {
	Name    string
	Version string
	Scope   platform.Scope
}

// RegistryView provides a merged view of packages across scopes.
type RegistryView struct {
	// packages maps package name to all installed instances across scopes.
	packages map[string][]InstalledPackage
}

// NewRegistryView builds a merged view from scope-ordered registries.
// scopes must be ordered from narrowest to broadest (local, project, user).
func NewRegistryView(registries map[platform.Scope]*registry.Registry) *RegistryView {
	rv := &RegistryView{packages: make(map[string][]InstalledPackage)}
	for scope, reg := range registries {
		for name, entry := range reg.Packages {
			rv.packages[name] = append(rv.packages[name], InstalledPackage{
				Name:    name,
				Version: entry.Version,
				Scope:   scope,
			})
		}
	}
	return rv
}

// visibleScopes returns the scopes that a package at the given scope can see.
// local can see local, project, user; project can see project, user; user can see user.
func visibleScopes(from platform.Scope) map[platform.Scope]bool {
	visible := map[platform.Scope]bool{from: true}
	switch from {
	case platform.ScopeLocal:
		visible[platform.ScopeProject] = true
		visible[platform.ScopeUser] = true
	case platform.ScopeProject:
		visible[platform.ScopeUser] = true
	}
	return visible
}

// findPackage looks for a package by name in scopes visible from fromScope.
// Returns the first match (narrowest scope first by convention).
func (rv *RegistryView) findPackage(name string, fromScope platform.Scope) (InstalledPackage, bool) {
	visible := visibleScopes(fromScope)
	instances := rv.packages[name]

	// Prefer narrowest scope: check in precedence order
	for _, s := range platform.ScopePrecedence() {
		if !visible[s] {
			continue
		}
		for _, inst := range instances {
			if inst.Scope == s {
				return inst, true
			}
		}
	}
	return InstalledPackage{}, false
}

// CheckPackage evaluates a single package's dependencies against the registry view.
func CheckPackage(m *manifest.Manifest, pkgScope platform.Scope, view *RegistryView) PackageCheckResult {
	result := PackageCheckResult{
		PackageName:  m.Name,
		PackageScope: pkgScope.String(),
		Version:      m.Version,
		AllSatisfied: true,
	}

	if len(m.Dependencies) == 0 {
		return result
	}

	for depName, constraint := range m.Dependencies {
		r := checkOneDep(depName, constraint, pkgScope, view)
		result.Results = append(result.Results, r)
		if r.Status != Satisfied {
			result.AllSatisfied = false
		}
	}

	return result
}

func checkOneDep(depName, constraint string, fromScope platform.Scope, view *RegistryView) Result {
	installed, found := view.findPackage(depName, fromScope)
	if !found {
		return Result{
			DependencyName: depName,
			Constraint:     constraint,
			Status:         Missing,
			Message:        "not installed",
		}
	}

	// Installed — check version constraint
	if constraint == "" {
		return Result{
			DependencyName:   depName,
			Constraint:       constraint,
			InstalledVersion: installed.Version,
			InstalledScope:   installed.Scope.String(),
			Status:           Satisfied,
		}
	}

	ok, err := CheckVersion(installed.Version, constraint)
	if err != nil {
		return Result{
			DependencyName:   depName,
			Constraint:       constraint,
			InstalledVersion: installed.Version,
			InstalledScope:   installed.Scope.String(),
			Status:           UnparseableConstraint,
			Message:          err.Error(),
		}
	}
	if !ok {
		return Result{
			DependencyName:   depName,
			Constraint:       constraint,
			InstalledVersion: installed.Version,
			InstalledScope:   installed.Scope.String(),
			Status:           VersionMismatch,
			Message:          "installed " + installed.Version + ", requires " + constraint,
		}
	}
	return Result{
		DependencyName:   depName,
		Constraint:       constraint,
		InstalledVersion: installed.Version,
		InstalledScope:   installed.Scope.String(),
		Status:           Satisfied,
	}
}

// ManifestLoader loads a manifest from a store path.
type ManifestLoader func(pkgDir string) (*manifest.Manifest, error)

// CheckAll evaluates all packages across the given scopes.
// storePathFn returns the store directory path for a package name at a given scope.
// loadManifest loads a manifest from a package directory.
func CheckAll(
	scopes []platform.Scope,
	registries map[platform.Scope]*registry.Registry,
	storePathFn func(scope platform.Scope, name string) string,
	loadManifest ManifestLoader,
) CheckAllResult {
	view := NewRegistryView(registries)
	result := CheckAllResult{AllSatisfied: true}

	// Build dependency graph for circular detection
	depGraph := make(map[string]map[string]bool)

	for _, scope := range scopes {
		reg := registries[scope]
		if reg == nil {
			continue
		}
		for name := range reg.Packages {
			storePath := storePathFn(scope, name)
			m, err := loadManifest(storePath)
			if err != nil {
				// Can't load manifest — skip this package
				continue
			}
			// Track dependency graph for circular detection
			if len(m.Dependencies) > 0 {
				deps := make(map[string]bool)
				for depName := range m.Dependencies {
					deps[depName] = true
				}
				depGraph[m.Name] = deps
			}

			pkgResult := CheckPackage(m, scope, view)
			result.Packages = append(result.Packages, pkgResult)
			if !pkgResult.AllSatisfied {
				result.AllSatisfied = false
			}
		}
	}

	// Detect circular dependencies (A→B and B→A)
	result.Warnings = detectCircularDeps(depGraph)

	return result
}

// detectCircularDeps finds pairs of packages that depend on each other.
func detectCircularDeps(depGraph map[string]map[string]bool) []string {
	var warnings []string
	seen := make(map[string]bool)
	for a, aDeps := range depGraph {
		for b := range aDeps {
			pairKey := a + "↔" + b
			reversePairKey := b + "↔" + a
			if seen[pairKey] || seen[reversePairKey] {
				continue
			}
			if bDeps, ok := depGraph[b]; ok && bDeps[a] {
				warnings = append(warnings, fmt.Sprintf("circular dependency detected: %s ↔ %s", a, b))
				seen[pairKey] = true
				seen[reversePairKey] = true
			}
		}
	}
	return warnings
}
