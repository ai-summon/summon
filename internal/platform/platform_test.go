package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeString(t *testing.T) {
	assert.Equal(t, "local", ScopeLocal.String())
	assert.Equal(t, "project", ScopeProject.String())
	assert.Equal(t, "user", ScopeUser.String())
	assert.Equal(t, "unknown", Scope(99).String())
}

func TestParseScope(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Scope
		wantErr string
	}{
		{name: "local", input: "local", want: ScopeLocal},
		{name: "project", input: "project", want: ScopeProject},
		{name: "user", input: "user", want: ScopeUser},
		{name: "empty defaults local", input: "", want: ScopeLocal},
		{name: "invalid", input: "workspace", wantErr: "Invalid scope value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScope(tt.input)
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

func TestScopePrecedence(t *testing.T) {
	assert.Equal(t, []Scope{ScopeLocal, ScopeProject, ScopeUser}, ScopePrecedence())
}

func TestAllAdapters(t *testing.T) {
	adapters := AllAdapters("/some/project")
	require.Len(t, adapters, 2)
}

func TestAllAdapters_Names(t *testing.T) {
	adapters := AllAdapters("/some/project")
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = a.Name()
	}
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "copilot")
}

func TestDetectActive(t *testing.T) {
	// DetectActive may return empty on CI where neither platform is installed.
	all := AllAdapters("/some/project")
	active := DetectActive("/some/project")
	assert.LessOrEqual(t, len(active), len(all))
}

func TestSupportsScope(t *testing.T) {
	mock := &MockCmdRunner{}
	claude := &ClaudeAdapter{ProjectDir: "/test", Runner: mock}
	copilot := &CopilotAdapter{ProjectDir: "/test", Runner: mock}

	assert.True(t, SupportsScope(claude, ScopeLocal))
	assert.True(t, SupportsScope(claude, ScopeProject))
	assert.True(t, SupportsScope(claude, ScopeUser))

	assert.False(t, SupportsScope(copilot, ScopeLocal))
	assert.False(t, SupportsScope(copilot, ScopeProject))
	assert.True(t, SupportsScope(copilot, ScopeUser))
}

func TestDetectedNames(t *testing.T) {
	// Just verify it returns a string slice and doesn't panic.
	names := DetectedNames("/some/project")
	assert.IsType(t, []string{}, names)
}

func TestCmdRunner_RealRunner(t *testing.T) {
	runner := &RealCmdRunner{}
	stdout, _, err := runner.Run("echo", "hello")
	require.NoError(t, err)
	assert.Contains(t, stdout, "hello")
}

func TestWithCmdRunner(t *testing.T) {
	mock := &MockCmdRunner{}
	adapters := AllAdapters("/test", WithCmdRunner(mock))
	require.Len(t, adapters, 2)
	// Verify the mock is wired in by checking adapter types.
	claude, ok := adapters[0].(*ClaudeAdapter)
	require.True(t, ok)
	assert.Equal(t, mock, claude.Runner)
}
