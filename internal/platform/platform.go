package platform

import (
	"fmt"
	"strings"
)

// Scope represents the installation scope for a plugin.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopeLocal   Scope = "local"
)

// ParseScope converts a string to a Scope, validating the value.
func ParseScope(s string) (Scope, error) {
	switch s {
	case "user", "":
		return ScopeUser, nil
	case "project":
		return ScopeProject, nil
	case "local":
		return ScopeLocal, nil
	default:
		return "", fmt.Errorf("invalid scope %q: must be one of: user, project, local", s)
	}
}

// InstalledPlugin represents a plugin installed on a specific platform.
type InstalledPlugin struct {
	Name     string `json:"name"`
	Source   string `json:"source,omitempty"`
	Platform string `json:"platform"`
	Scope    string `json:"scope,omitempty"`
}

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
	LookPath(name string) (string, error)
}

// Adapter defines the interface for platform-specific plugin operations.
type Adapter interface {
	Name() string
	Detect() bool
	SupportedScopes() []Scope
	Install(source string, scope Scope) error
	Uninstall(name string, scope Scope) error
	Update(name string, scope Scope) error
	ListInstalled(scope Scope) ([]InstalledPlugin, error)
}

// ValidateScope checks if a scope is supported by the adapter.
func ValidateScope(a Adapter, scope Scope) error {
	for _, s := range a.SupportedScopes() {
		if s == scope {
			return nil
		}
	}
	return fmt.Errorf("%s does not support scope %q; supported scopes: %v", a.Name(), scope, a.SupportedScopes())
}

// DetectAdapters returns all detected platform adapters.
func DetectAdapters(runner CommandRunner) []Adapter {
	adapters := []Adapter{
		NewCopilotAdapter(runner),
		NewClaudeAdapter(runner),
	}

	var detected []Adapter
	for _, a := range adapters {
		if a.Detect() {
			detected = append(detected, a)
		}
	}
	return detected
}

// FilterByTarget filters adapters to only include the specified target.
func FilterByTarget(adapters []Adapter, target string) ([]Adapter, error) {
	if target == "" {
		return adapters, nil
	}
	for _, a := range adapters {
		if a.Name() == target {
			return []Adapter{a}, nil
		}
	}
	return nil, fmt.Errorf("target %q not found; detected CLIs: %v", target, adapterNames(adapters))
}

func adapterNames(adapters []Adapter) []string {
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = a.Name()
	}
	return names
}

// cliError builds an error that includes the CLI's output when available.
func cliError(action string, output []byte, err error) error {
	msg := strings.TrimSpace(string(output))
	if msg != "" {
		return fmt.Errorf("%s failed: %s", action, msg)
	}
	return fmt.Errorf("%s failed: %w", action, err)
}
