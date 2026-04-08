package depcheck

import (
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
)

// FindReverseDeps scans all installed manifests across the given scopes to find
// packages that depend on targetName.
func FindReverseDeps(
	targetName string,
	scopes []platform.Scope,
	registries map[platform.Scope]*registry.Registry,
	storePathFn func(scope platform.Scope, name string) string,
	loadManifest ManifestLoader,
) []ReverseDependent {
	var deps []ReverseDependent

	for _, scope := range scopes {
		reg := registries[scope]
		if reg == nil {
			continue
		}
		for pkgName := range reg.Packages {
			if pkgName == targetName {
				continue
			}
			storePath := storePathFn(scope, pkgName)
			m, err := loadManifest(storePath)
			if err != nil || m == nil {
				continue
			}
			if constraint, ok := m.Dependencies[targetName]; ok {
				deps = append(deps, ReverseDependent{
					PackageName: pkgName,
					Scope:       scope.String(),
					Constraint:  constraint,
				})
			}
		}
	}

	return deps
}
