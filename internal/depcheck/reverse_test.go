package depcheck

import (
	"fmt"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestFindReverseDeps_HasDependents(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"base-lib": {Version: "1.0.0"},
			"pkg-a":    {Version: "1.0.0"},
			"pkg-b":    {Version: "2.0.0"},
		},
	})
	manifests := map[string]*manifest.Manifest{
		"pkg-a": {
			Name: "pkg-a", Version: "1.0.0",
			Dependencies: map[string]string{"base-lib": ">=1.0.0"},
		},
		"pkg-b": {
			Name: "pkg-b", Version: "2.0.0",
			Dependencies: map[string]string{"base-lib": "^1.0.0"},
		},
	}

	deps := FindReverseDeps(
		"base-lib",
		[]platform.Scope{platform.ScopeLocal},
		regs,
		func(scope platform.Scope, name string) string { return "/store/" + name },
		func(pkgDir string) (*manifest.Manifest, error) {
			for name, m := range manifests {
				if pkgDir == "/store/"+name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("not found")
		},
	)

	assert.Len(t, deps, 2)
}

func TestFindReverseDeps_NoDependents(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"standalone": {Version: "1.0.0"},
			"other-pkg":  {Version: "1.0.0"},
		},
	})
	manifests := map[string]*manifest.Manifest{
		"other-pkg": {
			Name: "other-pkg", Version: "1.0.0",
		},
	}

	deps := FindReverseDeps(
		"standalone",
		[]platform.Scope{platform.ScopeLocal},
		regs,
		func(scope platform.Scope, name string) string { return "/store/" + name },
		func(pkgDir string) (*manifest.Manifest, error) {
			for name, m := range manifests {
				if pkgDir == "/store/"+name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("not found")
		},
	)

	assert.Empty(t, deps)
}

func TestFindReverseDeps_CrossScope(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"pkg-a": {Version: "1.0.0"},
		},
		platform.ScopeUser: {
			"base-lib": {Version: "1.0.0"},
			"pkg-b":    {Version: "2.0.0"},
		},
	})
	manifests := map[string]*manifest.Manifest{
		"pkg-a": {
			Name: "pkg-a", Version: "1.0.0",
			Dependencies: map[string]string{"base-lib": ">=1.0.0"},
		},
		"pkg-b": {
			Name: "pkg-b", Version: "2.0.0",
			Dependencies: map[string]string{"base-lib": "^1.0.0"},
		},
	}

	deps := FindReverseDeps(
		"base-lib",
		[]platform.Scope{platform.ScopeLocal, platform.ScopeUser},
		regs,
		func(scope platform.Scope, name string) string { return "/store/" + scope.String() + "/" + name },
		func(pkgDir string) (*manifest.Manifest, error) {
			for name, m := range manifests {
				if pkgDir == "/store/local/"+name || pkgDir == "/store/user/"+name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("not found")
		},
	)

	assert.Len(t, deps, 2)
	// Verify scopes are different
	scopeSet := map[string]bool{}
	for _, d := range deps {
		scopeSet[d.Scope] = true
	}
	assert.True(t, scopeSet["local"])
	assert.True(t, scopeSet["user"])
}
