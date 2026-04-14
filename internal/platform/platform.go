// Package platform provides adapters for integrating summon with AI coding
// platforms such as Claude Code and GitHub Copilot. Each adapter delegates to
// the platform's own CLI for plugin management — summon never reads or writes
// any platform configuration file.
package platform

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Adapter defines the interface for platform-specific behavior.
// Implementations MUST NOT read or write any platform configuration files.
type Adapter interface {
	// Name returns the platform identifier (e.g., "claude", "copilot").
	Name() string

	// Detect returns true if the platform is installed on this machine.
	// Detection MUST use binary/directory presence only, not config file reads.
	Detect() bool

	// SupportedScopes returns the scopes this platform natively supports.
	SupportedScopes() []Scope

	// DiscoverPackage makes a package visible to the platform at the given scope
	// by delegating to the platform's CLI commands.
	DiscoverPackage(pkgPath string, pkgName string, scope Scope) error

	// RemovePackage removes a package from the platform's discovery at the given scope
	// by delegating to the platform's CLI commands.
	RemovePackage(pkgName string, scope Scope) error

	// CleanOrphans removes artifacts for this platform if it is no longer detected.
	CleanOrphans() error
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
		return 0, fmt.Errorf("Invalid scope value %q. Allowed: local, project, user.", value)
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
	cmdRunner CmdRunner
}

// WithCmdRunner overrides the command runner used by adapters.
// This is primarily for testing to mock CLI invocations.
func WithCmdRunner(runner CmdRunner) AdapterOption {
	return func(c *adapterConfig) {
		c.cmdRunner = runner
	}
}

// AllAdapters returns all known platform adapters.
func AllAdapters(projectDir string, opts ...AdapterOption) []Adapter {
	cfg := &adapterConfig{}
	for _, o := range opts {
		o(cfg)
	}
	runner := cfg.cmdRunner
	if runner == nil {
		runner = &RealCmdRunner{}
	}
	return []Adapter{
		&ClaudeAdapter{ProjectDir: projectDir, Runner: runner},
		&CopilotAdapter{ProjectDir: projectDir, Runner: runner},
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

// DetectedNames returns the names of all detected platforms.
func DetectedNames(projectDir string, opts ...AdapterOption) []string {
	var names []string
	for _, a := range DetectActive(projectDir, opts...) {
		names = append(names, a.Name())
	}
	return names
}

// SupportsScope returns true if the given scope is in the adapter's supported scopes list.
func SupportsScope(a Adapter, scope Scope) bool {
	for _, s := range a.SupportedScopes() {
		if s == scope {
			return true
		}
	}
	return false
}

// CmdRunner abstracts command execution for testability.
type CmdRunner interface {
	Run(name string, args ...string) (stdout string, stderr string, err error)
}

// RealCmdRunner executes commands via os/exec with a timeout.
type RealCmdRunner struct{}

const cmdTimeout = 60 * time.Second

func (r *RealCmdRunner) Run(name string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return outBuf.String(), errBuf.String(),
			fmt.Errorf("command %q failed: %w\nstderr: %s", name+" "+joinArgs(args), err, errBuf.String())
	}
	return outBuf.String(), errBuf.String(), nil
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
