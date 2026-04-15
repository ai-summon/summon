package selfmgmt

import (
	"fmt"
	"os"
	"path/filepath"
)

// SummonPaths holds the runtime-resolved paths for the summon installation.
type SummonPaths struct {
	BinaryPath string // Absolute path to the summon binary, resolved through symlinks
	BinaryDir  string // Directory containing the binary
	ConfigDir  string // Summon configuration directory (~/.summon/)
}

// PathResolver abstracts OS-level path resolution for testability.
type PathResolver interface {
	Executable() (string, error)
	EvalSymlinks(path string) (string, error)
	UserHomeDir() (string, error)
}

type osPathResolver struct{}

func (o *osPathResolver) Executable() (string, error) {
	return os.Executable()
}

func (o *osPathResolver) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (o *osPathResolver) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// ResolvePaths determines the binary path, binary directory, and config directory
// using OS-level APIs. Uses the default OS path resolver.
func ResolvePaths() (SummonPaths, error) {
	return ResolvePathsWith(&osPathResolver{})
}

// ResolvePathsWith determines the binary path, binary directory, and config directory
// using the provided PathResolver (for testability).
func ResolvePathsWith(resolver PathResolver) (SummonPaths, error) {
	exePath, err := resolver.Executable()
	if err != nil {
		return SummonPaths{}, fmt.Errorf("cannot determine binary path: %w", err)
	}

	realPath, err := resolver.EvalSymlinks(exePath)
	if err != nil {
		return SummonPaths{}, fmt.Errorf("cannot resolve symlinks for binary path: %w", err)
	}

	homeDir, err := resolver.UserHomeDir()
	if err != nil {
		return SummonPaths{}, fmt.Errorf("cannot determine home directory: %w", err)
	}

	return SummonPaths{
		BinaryPath: realPath,
		BinaryDir:  filepath.Dir(realPath),
		ConfigDir:  filepath.Join(homeDir, ".summon"),
	}, nil
}
