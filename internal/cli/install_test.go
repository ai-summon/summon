package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallCmd_Flags(t *testing.T) {
	flags := installCmd.Flags()
	for _, name := range []string{"path", "global", "project", "scope", "ref", "force"} {
		assert.NotNil(t, flags.Lookup(name), "install should have --%s flag", name)
	}
}

func TestResolveInstallScope(t *testing.T) {
	tests := []struct {
		name      string
		scopeFlag string
		global    bool
		project   bool
		want      platform.Scope
		wantErr   string
	}{
		{name: "default local", want: platform.ScopeLocal},
		{name: "legacy global becomes user", global: true, want: platform.ScopeUser},
		{name: "explicit project", scopeFlag: "project", want: platform.ScopeProject},
		{name: "explicit user overrides global", scopeFlag: "user", global: false, want: platform.ScopeUser},
		{name: "explicit local overrides global", scopeFlag: "local", global: true, want: platform.ScopeLocal},
		{name: "project flag", project: true, want: platform.ScopeProject},
		{name: "invalid", scopeFlag: "workspace", wantErr: "Invalid scope value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInstallScope(tt.scopeFlag, tt.global, tt.project)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveRestoreScope(t *testing.T) {
	tests := []struct {
		name      string
		scopeFlag string
		global    bool
		project   bool
		want      platform.Scope
	}{
		{name: "default project", want: platform.ScopeProject},
		{name: "explicit scope overrides default", scopeFlag: "local", want: platform.ScopeLocal},
		{name: "global flag", global: true, want: platform.ScopeUser},
		{name: "project flag", project: true, want: platform.ScopeProject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRestoreScope(tt.scopeFlag, tt.global, tt.project)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunInstall_NoArgs_LocalScopeReturnsError(t *testing.T) {
	_ = setupProjectDir(t)

	installPath = ""
	installGlobal = false
	installScope = "local"
	installRef = ""
	installForce = false

	err := runInstall(installCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local installs are not restored automatically")
}

func TestRunInstall_NoArgs_ProjectScopeUsesProjectRegistry(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "project", `
summon_version: "0.1.0"
packages: {}
`)

	installPath = ""
	installGlobal = false
	installScope = "project"
	installRef = ""
	installForce = false

	out := captureStdout(t, func() {
		err := runInstall(installCmd, []string{})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages to restore")
}

func TestInstallCmd_ArgsValidator(t *testing.T) {
	assert.NoError(t, installCmd.Args(installCmd, []string{"one"}))
	assert.NoError(t, installCmd.Args(installCmd, []string{}))
	assert.Error(t, installCmd.Args(installCmd, []string{"a", "b"}),
		"install should reject more than 1 positional arg")
}

func TestInstallCmd_MutuallyExclusiveFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "scope and global", args: []string{"install", "--scope", "local", "--global", "pkg"}},
		{name: "scope and project", args: []string{"install", "--scope", "local", "--project", "pkg"}},
		{name: "global and project", args: []string{"install", "--global", "--project", "pkg"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flag state between subtests (Cobra retains "changed" status).
			t.Cleanup(func() {
				installCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
			})

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "if any flags in the group [scope global project] are set none of the others can be")
		})
	}
}

func TestInstallCmd_InvalidScope(t *testing.T) {
	t.Cleanup(func() {
		installCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	})

	rootCmd.SetArgs([]string{"install", "--scope", "system", "pkg"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid scope value")
}

func TestRunInstall_NoArgs_EmptyRegistry(t *testing.T) {
	_ = setupProjectDir(t)

	installPath = ""
	installGlobal = false
	installScope = ""
	installRef = ""
	installForce = false

	out := captureStdout(t, func() {
		err := runInstall(installCmd, []string{})
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No packages to restore")
}

func TestInstallCmd_StrictDepsFlag(t *testing.T) {
	f := installCmd.Flags().Lookup("strict-deps")
	require.NotNil(t, f, "install should have --strict-deps flag")
	assert.Equal(t, "false", f.DefValue, "--strict-deps should default to false")
}

func TestRunInstall_StrictDepsWiredToOptions(t *testing.T) {
	// Verify the installStrictDeps variable is passed through to opts.StrictDeps.
	// Set the flag, attempt an install with a nonexistent local path (which will
	// fail during install), and ensure the error is path-related not flag-related.
	_ = setupProjectDir(t)

	installPath = "/nonexistent-path-for-flag-test"
	installGlobal = false
	installProject = false
	installScope = ""
	installRef = ""
	installForce = false
	installStrictDeps = true
	t.Cleanup(func() {
		installStrictDeps = false
		installPath = ""
	})

	err := runInstall(installCmd, []string{})
	require.Error(t, err)
	// The error should be about the path/package, not about the flag itself
	assert.NotContains(t, err.Error(), "strict-deps")
}

func TestRunInstall_StrictDeps_LocalPathMissingDeps(t *testing.T) {
	dir := setupProjectDir(t)

	// Create a package with missing deps at a local path
	pkgDir := filepath.Join(dir, "test-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "summon.yaml"), []byte(`
name: test-strict
version: "1.0.0"
description: "test strict deps"
platforms: []
dependencies:
  nonexistent-dep: ">=1.0.0"
`), 0o644))

	installPath = pkgDir
	installGlobal = false
	installProject = false
	installScope = ""
	installRef = ""
	installForce = true // bypass platform check
	installStrictDeps = true
	t.Cleanup(func() {
		installStrictDeps = false
		installForce = false
		installPath = ""
	})

	var buf bytes.Buffer
	oldStderr := installer.Stderr
	installer.Stderr = &buf
	t.Cleanup(func() { installer.Stderr = oldStderr })

	err := runInstall(installCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required dependencies")
}

func TestRunInstall_DefaultMode_MissingDeps_WarnAndContinue(t *testing.T) {
	dir := setupProjectDir(t)

	// Create a package with missing deps at a local path
	pkgDir := filepath.Join(dir, "test-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "summon.yaml"), []byte(`
name: test-warn
version: "1.0.0"
description: "test warn and continue"
platforms: []
dependencies:
  nonexistent-dep: ">=1.0.0"
`), 0o644))

	// Set SUMMON_NONINTERACTIVE to prevent prompt
	t.Setenv("SUMMON_NONINTERACTIVE", "1")

	installPath = pkgDir
	installGlobal = false
	installProject = false
	installScope = ""
	installRef = ""
	installForce = true // bypass platform check
	installStrictDeps = false
	t.Cleanup(func() {
		installForce = false
		installPath = ""
	})

	var buf bytes.Buffer
	oldStderr := installer.Stderr
	installer.Stderr = &buf
	t.Cleanup(func() { installer.Stderr = oldStderr })

	out := captureStdout(t, func() {
		err := runInstall(installCmd, []string{})
		// Without --strict-deps, install should succeed despite missing deps
		assert.NoError(t, err)
	})

	// Should show installed message (warn-and-continue behavior preserved)
	assert.Contains(t, out, "Installed")
	// Should warn about missing deps
	assert.Contains(t, buf.String(), "missing dependencies")
}
