package depcheck

import (
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeRegistries(entries map[platform.Scope]map[string]registry.Entry) map[platform.Scope]*registry.Registry {
	regs := make(map[platform.Scope]*registry.Registry)
	for scope, pkgs := range entries {
		reg := &registry.Registry{Packages: make(map[string]registry.Entry)}
		for name, entry := range pkgs {
			reg.Packages[name] = entry
		}
		regs[scope] = reg
	}
	return regs
}

func TestCheckPackage_AllSatisfied(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
			"dep-a":  {Version: "2.0.0"},
			"dep-b":  {Version: "1.5.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"dep-a": ">=1.0.0",
			"dep-b": "^1.0.0",
		},
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.True(t, result.AllSatisfied)
	assert.Len(t, result.Results, 2)
	for _, r := range result.Results {
		assert.Equal(t, Satisfied, r.Status)
	}
}

func TestCheckPackage_MissingDep(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"missing-dep": ">=1.0.0",
		},
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.False(t, result.AllSatisfied)
	require.Len(t, result.Results, 1)
	assert.Equal(t, Missing, result.Results[0].Status)
	assert.Equal(t, "not installed", result.Results[0].Message)
}

func TestCheckPackage_VersionMismatch(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
			"dep-a":  {Version: "1.5.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"dep-a": ">=2.0.0",
		},
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.False(t, result.AllSatisfied)
	require.Len(t, result.Results, 1)
	assert.Equal(t, VersionMismatch, result.Results[0].Status)
	assert.Contains(t, result.Results[0].Message, "installed 1.5.0")
}

func TestCheckPackage_EmptyConstraint(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
			"dep-a":  {Version: "0.1.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"dep-a": "", // any version
		},
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.True(t, result.AllSatisfied)
	require.Len(t, result.Results, 1)
	assert.Equal(t, Satisfied, result.Results[0].Status)
}

func TestCheckPackage_UnparseableConstraint(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
			"dep-a":  {Version: "1.0.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"dep-a": "not-valid-constraint",
		},
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.False(t, result.AllSatisfied)
	require.Len(t, result.Results, 1)
	assert.Equal(t, UnparseableConstraint, result.Results[0].Status)
}

func TestCheckPackage_CrossScopeSatisfaction(t *testing.T) {
	// Package at local scope depends on dep at user scope — should be satisfied
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
		},
		platform.ScopeUser: {
			"dep-a": {Version: "2.0.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"dep-a": ">=1.0.0",
		},
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.True(t, result.AllSatisfied)
	require.Len(t, result.Results, 1)
	assert.Equal(t, Satisfied, result.Results[0].Status)
	assert.Equal(t, "user", result.Results[0].InstalledScope)
}

func TestCheckPackage_UserScopeCannotSeeLocal(t *testing.T) {
	// Package at user scope depends on dep at local scope — NOT satisfied
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeUser: {
			"my-pkg": {Version: "1.0.0"},
		},
		platform.ScopeLocal: {
			"dep-a": {Version: "2.0.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
		Dependencies: map[string]string{
			"dep-a": ">=1.0.0",
		},
	}

	result := CheckPackage(m, platform.ScopeUser, view)
	assert.False(t, result.AllSatisfied)
	require.Len(t, result.Results, 1)
	assert.Equal(t, Missing, result.Results[0].Status)
}

func TestCheckPackage_NoDependencies(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"my-pkg": {Version: "1.0.0"},
		},
	})
	view := NewRegistryView(regs)
	m := &manifest.Manifest{
		Name:    "my-pkg",
		Version: "1.0.0",
	}

	result := CheckPackage(m, platform.ScopeLocal, view)
	assert.True(t, result.AllSatisfied)
	assert.Empty(t, result.Results)
}

func TestCheckAll_NoPackages(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {},
	})
	result := CheckAll(
		[]platform.Scope{platform.ScopeLocal},
		regs,
		func(scope platform.Scope, name string) string { return "/tmp/" + name },
		func(pkgDir string) (*manifest.Manifest, error) {
			return nil, nil
		},
	)
	assert.True(t, result.AllSatisfied)
	assert.Empty(t, result.Packages)
}

func TestCheckAll_MixedResults(t *testing.T) {
	regs := makeRegistries(map[platform.Scope]map[string]registry.Entry{
		platform.ScopeLocal: {
			"pkg-a": {Version: "1.0.0"},
			"pkg-b": {Version: "2.0.0"},
			"dep-x": {Version: "1.0.0"},
		},
	})
	manifests := map[string]*manifest.Manifest{
		"pkg-a": {
			Name: "pkg-a", Version: "1.0.0",
			Dependencies: map[string]string{"dep-x": ">=1.0.0"},
		},
		"pkg-b": {
			Name: "pkg-b", Version: "2.0.0",
			Dependencies: map[string]string{"dep-missing": ">=1.0.0"},
		},
		"dep-x": {
			Name: "dep-x", Version: "1.0.0",
		},
	}

	result := CheckAll(
		[]platform.Scope{platform.ScopeLocal},
		regs,
		func(scope platform.Scope, name string) string { return "/store/" + name },
		func(pkgDir string) (*manifest.Manifest, error) {
			// Extract name from path
			for name, m := range manifests {
				if pkgDir == "/store/"+name {
					return m, nil
				}
			}
			return nil, nil
		},
	)
	assert.False(t, result.AllSatisfied)
	assert.Len(t, result.Packages, 3)
}
