package platform

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotAdapter_Name(t *testing.T) {
	a := &CopilotAdapter{Runner: &MockCmdRunner{}}
	assert.Equal(t, "copilot", a.Name())
}

func TestCopilotAdapter_SupportedScopes(t *testing.T) {
	a := &CopilotAdapter{Runner: &MockCmdRunner{}}
	scopes := a.SupportedScopes()
	assert.Equal(t, []Scope{ScopeUser}, scopes)
	assert.Len(t, scopes, 1)
}

func TestCopilotAdapter_SupportedScopes_NoLocalOrProject(t *testing.T) {
	a := &CopilotAdapter{Runner: &MockCmdRunner{}}
	assert.False(t, SupportsScope(a, ScopeLocal))
	assert.False(t, SupportsScope(a, ScopeProject))
	assert.True(t, SupportsScope(a, ScopeUser))
}

func TestCopilotAdapter_DiscoverPackage(t *testing.T) {
	mock := &MockCmdRunner{}
	a := &CopilotAdapter{ProjectDir: "/project", Runner: mock}

	err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeUser)
	require.NoError(t, err)

	// Should have exactly 1 call: direct plugin install (no marketplace add needed).
	require.Len(t, mock.Calls, 1)
	c := mock.Calls[0]
	assert.Equal(t, "copilot", c.Name)
	assert.Equal(t, []string{"plugin", "install", "/store/my-plugin"}, c.Args)
}

func TestCopilotAdapter_DiscoverPackage_Fails(t *testing.T) {
	mock := &MockCmdRunner{Err: fmt.Errorf("cli error")}
	a := &CopilotAdapter{ProjectDir: "/project", Runner: mock}

	err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeUser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copilot plugin install")
}

func TestCopilotAdapter_RemovePackage(t *testing.T) {
	mock := &MockCmdRunner{}
	a := &CopilotAdapter{ProjectDir: "/project", Runner: mock}

	err := a.RemovePackage("my-plugin", ScopeUser)
	require.NoError(t, err)

	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "copilot", mock.Calls[0].Name)
	assert.Equal(t, []string{"plugin", "uninstall", "my-plugin"}, mock.Calls[0].Args)
}

func TestCopilotAdapter_RemovePackage_FailsGracefully(t *testing.T) {
	mock := &MockCmdRunner{Err: fmt.Errorf("uninstall error")}
	a := &CopilotAdapter{ProjectDir: "/project", Runner: mock}

	// RemovePackage should not return an error even if CLI fails.
	err := a.RemovePackage("my-plugin", ScopeUser)
	require.NoError(t, err)
}

func TestCopilotAdapter_CleanOrphans(t *testing.T) {
	a := &CopilotAdapter{Runner: &MockCmdRunner{}}
	err := a.CleanOrphans()
	require.NoError(t, err)
}

func TestCopilotAdapter_DirectInstall_NoMarketplaceStep(t *testing.T) {
	mock := &MockCmdRunner{}
	a := &CopilotAdapter{ProjectDir: "/project", Runner: mock}

	err := a.DiscoverPackage("github:user/repo", "repo", ScopeUser)
	require.NoError(t, err)

	// Copilot uses direct install — only 1 call, no marketplace add.
	require.Len(t, mock.Calls, 1)
	assert.NotContains(t, mock.Calls[0].Args, "marketplace")
}
