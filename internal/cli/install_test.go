package cli

import (
	"testing"

	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallCmd_Flags(t *testing.T) {
	flags := installCmd.Flags()
	for _, name := range []string{"path", "global", "scope", "ref", "force"} {
		assert.NotNil(t, flags.Lookup(name), "install should have --%s flag", name)
	}
}

func TestResolveInstallScope(t *testing.T) {
	tests := []struct {
		name      string
		scopeFlag string
		global    bool
		want      platform.Scope
		wantErr   string
	}{
		{name: "default local", want: platform.ScopeLocal},
		{name: "legacy global becomes user", global: true, want: platform.ScopeUser},
		{name: "explicit project", scopeFlag: "project", want: platform.ScopeProject},
		{name: "explicit user overrides global", scopeFlag: "user", global: false, want: platform.ScopeUser},
		{name: "explicit local overrides global", scopeFlag: "local", global: true, want: platform.ScopeLocal},
		{name: "invalid", scopeFlag: "workspace", wantErr: "invalid scope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInstallScope(tt.scopeFlag, tt.global)
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
