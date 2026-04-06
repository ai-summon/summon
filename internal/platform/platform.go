// Package platform provides adapters for integrating summon with AI coding
// platforms such as Claude Code and VS Code Copilot. Each adapter knows how
// to detect the platform, locate its settings file, and register or
// unregister summon marketplaces.
package platform

import "fmt"

// Adapter defines the interface for platform-specific behavior.
type Adapter interface {
	Name() string
	Detect() bool
	SettingsPath(scope Scope) string
	Register(marketplacePath string, marketplaceName string, scope Scope) error
	Unregister(marketplaceName string, scope Scope) error
	EnablePlugin(pluginName string, marketplaceName string, storeDir string, scope Scope) error
	DisablePlugin(pluginName string, marketplaceName string, storeDir string, scope Scope) error
}

// Materializer is an optional extension to Adapter that supports workspace
// component materialization for project and local scopes. Adapters implementing
// this interface can create and remove workspace-visible symlinks for skills and
// agents into documented customization paths.
type Materializer interface {
	MaterializeComponents(pkgDir string, m ComponentsInfo, scope Scope) error
	RemoveMaterialized(pkgName string, m ComponentsInfo, scope Scope) error
}

// Scope represents a writable installation scope.
type Scope int

const (
	ScopeLocal Scope = iota
	ScopeProject
	ScopeUser

	// ScopeGlobal is kept as a compatibility alias for older global/local call
	// sites. New code should prefer ScopeUser.
	ScopeGlobal = ScopeUser
)

func (s Scope) String() string {
	switch s {
	case ScopeLocal:
		return "local"
	case ScopeProject:
		return "project"
	case ScopeUser:
		return "user"
	default:
		return "unknown"
	}
}

// ParseScope converts CLI/user input into a writable scope. An empty value
// maps to the default install scope.
func ParseScope(value string) (Scope, error) {
	switch value {
	case "", ScopeLocal.String():
		return ScopeLocal, nil
	case ScopeProject.String():
		return ScopeProject, nil
	case ScopeUser.String(), "global":
		return ScopeUser, nil
	default:
		return 0, fmt.Errorf("invalid scope %q (supported: user, project, local)", value)
	}
}

// ScopePrecedence returns the effective visibility order when a package exists
// in multiple writable scopes.
func ScopePrecedence() []Scope {
	return []Scope{ScopeLocal, ScopeProject, ScopeUser}
}

// AdapterOption configures adapter construction.
type AdapterOption func(*adapterConfig)

type adapterConfig struct {
	globalSettingsDir string
}

// WithGlobalSettingsDir overrides the VS Code user settings directory
// on the CopilotAdapter. This is used in tests to avoid writing to
// real user settings.
func WithGlobalSettingsDir(dir string) AdapterOption {
	return func(c *adapterConfig) {
		c.globalSettingsDir = dir
	}
}

// AllAdapters returns all known platform adapters.
func AllAdapters(projectDir string, opts ...AdapterOption) []Adapter {
	cfg := &adapterConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return []Adapter{
		&ClaudeAdapter{ProjectDir: projectDir},
		&CopilotAdapter{ProjectDir: projectDir, GlobalSettingsDir: cfg.globalSettingsDir},
	}
}

// DetectActive returns adapters for platforms that are installed on this system.
func DetectActive(projectDir string, opts ...AdapterOption) []Adapter {
	var active []Adapter
	for _, a := range AllAdapters(projectDir, opts...) {
		if a.Detect() {
			active = append(active, a)
		}
	}
	return active
}
