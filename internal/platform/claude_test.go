package platform

import (
"fmt"
"testing"

"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
)

func TestClaudeAdapter_Name(t *testing.T) {
a := &ClaudeAdapter{Runner: &MockCmdRunner{}}
assert.Equal(t, "claude", a.Name())
}

func TestClaudeAdapter_SupportedScopes(t *testing.T) {
a := &ClaudeAdapter{Runner: &MockCmdRunner{}}
scopes := a.SupportedScopes()
assert.Contains(t, scopes, ScopeLocal)
assert.Contains(t, scopes, ScopeProject)
assert.Contains(t, scopes, ScopeUser)
assert.Len(t, scopes, 3)
}

func TestClaudeAdapter_DiscoverPackage_UserScope(t *testing.T) {
mock := &MockCmdRunner{}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeUser)
require.NoError(t, err)

// Should have 2 calls: marketplace add, then plugin install.
require.Len(t, mock.Calls, 2)

// First call: marketplace add.
c1 := mock.Calls[0]
assert.Equal(t, "claude", c1.Name)
assert.Contains(t, c1.Args, "marketplace")
assert.Contains(t, c1.Args, "add")
assert.Contains(t, c1.Args, "--scope")
assert.Contains(t, c1.Args, "user")

// Second call: plugin install.
c2 := mock.Calls[1]
assert.Equal(t, "claude", c2.Name)
assert.Contains(t, c2.Args, "install")
assert.Contains(t, c2.Args, "--scope")
assert.Contains(t, c2.Args, "user")
}

func TestClaudeAdapter_DiscoverPackage_LocalScope(t *testing.T) {
mock := &MockCmdRunner{}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeLocal)
require.NoError(t, err)

require.Len(t, mock.Calls, 2)
assert.Contains(t, mock.Calls[0].Args, "local")
assert.Contains(t, mock.Calls[1].Args, "local")
}

func TestClaudeAdapter_DiscoverPackage_ProjectScope(t *testing.T) {
mock := &MockCmdRunner{}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeProject)
require.NoError(t, err)

require.Len(t, mock.Calls, 2)
assert.Contains(t, mock.Calls[0].Args, "project")
assert.Contains(t, mock.Calls[1].Args, "project")
}

func TestClaudeAdapter_DiscoverPackage_MarketplaceAddFails(t *testing.T) {
mock := &MockCmdRunner{Err: fmt.Errorf("cli error")}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeUser)
require.Error(t, err)
assert.Contains(t, err.Error(), "claude marketplace add")
}

func TestClaudeAdapter_DiscoverPackage_InstallFails(t *testing.T) {
mock := &MockCmdRunner{
CallErrors: map[string]error{
"claude plugin install my-plugin@my-plugin --scope user": fmt.Errorf("install failed"),
},
}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.DiscoverPackage("/store/my-plugin", "my-plugin", ScopeUser)
require.Error(t, err)
assert.Contains(t, err.Error(), "claude plugin install")
}

func TestClaudeAdapter_RemovePackage(t *testing.T) {
mock := &MockCmdRunner{}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.RemovePackage("my-plugin", ScopeUser)
require.NoError(t, err)

require.Len(t, mock.Calls, 1)
assert.Equal(t, "claude", mock.Calls[0].Name)
assert.Contains(t, mock.Calls[0].Args, "uninstall")
assert.Contains(t, mock.Calls[0].Args, "my-plugin")
}

func TestClaudeAdapter_RemovePackage_FailsGracefully(t *testing.T) {
mock := &MockCmdRunner{Err: fmt.Errorf("uninstall error")}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

// RemovePackage should not return an error even if CLI fails.
err := a.RemovePackage("my-plugin", ScopeUser)
require.NoError(t, err)
}

func TestClaudeAdapter_CleanOrphans(t *testing.T) {
a := &ClaudeAdapter{Runner: &MockCmdRunner{}}
err := a.CleanOrphans()
require.NoError(t, err)
}

func TestClaudeAdapter_PluginRefFormat(t *testing.T) {
mock := &MockCmdRunner{}
a := &ClaudeAdapter{ProjectDir: "/project", Runner: mock}

err := a.DiscoverPackage("/store/wingman", "wingman", ScopeUser)
require.NoError(t, err)

// The install call should use "pkgName@marketplaceName" format.
installCall := mock.Calls[1]
found := false
for _, arg := range installCall.Args {
if arg == "wingman@wingman" {
found = true
break
}
}
assert.True(t, found, "install call should use pkgName@marketplaceName format, got args: %v", installCall.Args)
}
