package cli

import (
	"fmt"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
)

func resolveQueryScopes(scopeFlag string, global bool, project bool) ([]platform.Scope, error) {
	if scopeFlag != "" || global || project {
		scope, err := resolveInstallScope(scopeFlag, global, project)
		if err != nil {
			return nil, err
		}
		return []platform.Scope{scope}, nil
	}
	return platform.ScopePrecedence(), nil
}

func findInstalledScopes(projectDir, packageName string) ([]platform.Scope, error) {
	var matches []platform.Scope
	for _, scope := range platform.ScopePrecedence() {
		paths := installer.ResolvePaths(scope, projectDir)
		reg, err := registry.Load(paths.RegistryPath)
		if err != nil {
			return nil, fmt.Errorf("loading %s registry: %w", scope.String(), err)
		}
		if reg.Has(packageName) {
			matches = append(matches, scope)
		}
	}
	return matches, nil
}

func resolveExistingPackageScope(projectDir, packageName, scopeFlag string, global bool, project bool) (platform.Scope, error) {
	if scopeFlag != "" || global || project {
		scope, err := resolveInstallScope(scopeFlag, global, project)
		if err != nil {
			return 0, err
		}
		paths := installer.ResolvePaths(scope, projectDir)
		reg, err := registry.Load(paths.RegistryPath)
		if err != nil {
			return 0, fmt.Errorf("loading %s registry: %w", scope.String(), err)
		}
		if !reg.Has(packageName) {
			return 0, fmt.Errorf("package %q is not installed in %s scope", packageName, scope.String())
		}
		return scope, nil
	}

	matches, err := findInstalledScopes(projectDir, packageName)
	if err != nil {
		return 0, err
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("package %q is not installed", packageName)
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("package %q is installed in multiple scopes (%s); rerun with --scope", packageName, joinScopeNames(matches))
	}
	return matches[0], nil
}

func joinScopeNames(scopes []platform.Scope) string {
	parts := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		parts = append(parts, scope.String())
	}
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += ", " + parts[i]
	}
	return result
}
