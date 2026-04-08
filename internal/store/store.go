// Package store manages the on-disk package store directory where installed
// packages are kept as subdirectories or symlinks, providing operations to
// link, list, remove, and inspect package entries.
package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ai-summon/summon/internal/fsutil"
)

// Store manages the package store directory.
type Store struct {
	Dir string
}

// New creates a Store for the given base directory.
func New(dir string) *Store {
	return &Store{Dir: dir}
}

// Init ensures the store directory exists.
func (s *Store) Init() error {
	return os.MkdirAll(s.Dir, 0o755)
}

// PackagePath returns the path to a package in the store.
func (s *Store) PackagePath(name string) string {
	return filepath.Join(s.Dir, name)
}

// Link creates a symlink from the store to the given source directory.
func (s *Store) Link(name, source string) error {
	if err := s.Init(); err != nil {
		return err
	}
	target := s.PackagePath(name)
	// Remove existing if present
	if _, err := os.Lstat(target); err == nil {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("removing existing store entry: %w", err)
		}
	}
	absSource, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolving source path: %w", err)
	}
	return fsutil.CreateDirLink(absSource, target)
}

// MoveFromStage moves a staged package directory into the store, replacing any
// existing entry with the same package name.
func (s *Store) MoveFromStage(name, source string) error {
	if err := s.Init(); err != nil {
		return err
	}
	if err := s.Remove(name); err != nil {
		return fmt.Errorf("removing existing store entry: %w", err)
	}
	target := s.PackagePath(name)
	if err := fsutil.MoveDir(source, target); err != nil {
		return fmt.Errorf("moving staged package: %w", err)
	}
	return nil
}

// Has checks if a package exists in the store.
func (s *Store) Has(name string) bool {
	_, err := os.Lstat(s.PackagePath(name))
	return err == nil
}

// Remove removes a package from the store.
func (s *Store) Remove(name string) error {
	path := s.PackagePath(name)
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fsutil.IsLink(path) {
		return fsutil.RemoveLink(path)
	}
	return os.RemoveAll(path)
}

// List lists all packages in the store.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// IsBrokenLink checks if a store entry is a broken symlink or junction.
func (s *Store) IsBrokenLink(name string) bool {
	path := s.PackagePath(name)
	if !fsutil.IsLink(path) {
		return false
	}
	_, err := os.Stat(path)
	return err != nil
}
