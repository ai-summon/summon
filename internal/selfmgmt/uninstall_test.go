package selfmgmt

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFileSystem struct {
	removedAll []string
	removed    []string
	statPaths  map[string]bool // true = exists
	removeErr  map[string]error
}

func newFakeFileSystem() *fakeFileSystem {
	return &fakeFileSystem{
		statPaths: make(map[string]bool),
		removeErr: make(map[string]error),
	}
}

func (f *fakeFileSystem) RemoveAll(path string) error {
	if err, ok := f.removeErr[path]; ok {
		return err
	}
	f.removedAll = append(f.removedAll, path)
	return nil
}

func (f *fakeFileSystem) Remove(path string) error {
	if err, ok := f.removeErr[path]; ok {
		return err
	}
	f.removed = append(f.removed, path)
	return nil
}

func (f *fakeFileSystem) Stat(path string) (os.FileInfo, error) {
	if exists, ok := f.statPaths[path]; ok && exists {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

func TestUninstallWith_BothArtifactsRemoved(t *testing.T) {
	fs := newFakeFileSystem()
	fs.statPaths["/home/user/.summon"] = true
	paths := SummonPaths{
		BinaryPath: "/home/user/.local/bin/summon",
		BinaryDir:  "/home/user/.local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	err := UninstallWith(paths, &buf, fs)
	require.NoError(t, err)
	assert.Equal(t, []string{"/home/user/.summon"}, fs.removedAll)
	assert.Equal(t, []string{"/home/user/.local/bin/summon"}, fs.removed)
	assert.Contains(t, buf.String(), "removed /home/user/.summon")
	assert.Contains(t, buf.String(), "removed /home/user/.local/bin/summon")
}

func TestUninstallWith_ConfigDirMissing(t *testing.T) {
	fs := newFakeFileSystem()
	// Config dir does NOT exist
	paths := SummonPaths{
		BinaryPath: "/home/user/.local/bin/summon",
		BinaryDir:  "/home/user/.local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	err := UninstallWith(paths, &buf, fs)
	require.NoError(t, err)
	assert.Empty(t, fs.removedAll, "should not attempt to remove missing config dir")
	assert.Equal(t, []string{"/home/user/.local/bin/summon"}, fs.removed)
	assert.NotContains(t, buf.String(), "removed /home/user/.summon")
	assert.Contains(t, buf.String(), "removed /home/user/.local/bin/summon")
}

func TestUninstallWith_BinaryPermissionError(t *testing.T) {
	fs := newFakeFileSystem()
	fs.removeErr["/home/user/.local/bin/summon"] = os.ErrPermission
	paths := SummonPaths{
		BinaryPath: "/home/user/.local/bin/summon",
		BinaryDir:  "/home/user/.local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	err := UninstallWith(paths, &buf, fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Contains(t, err.Error(), "elevated permissions")
}

func TestUninstallWith_ConfigDirPermissionError(t *testing.T) {
	fs := newFakeFileSystem()
	fs.statPaths["/home/user/.summon"] = true
	fs.removeErr["/home/user/.summon"] = fmt.Errorf("permission denied")
	paths := SummonPaths{
		BinaryPath: "/home/user/.local/bin/summon",
		BinaryDir:  "/home/user/.local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	err := UninstallWith(paths, &buf, fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestUninstallWith_SystemWideBinaryWarning(t *testing.T) {
	fs := newFakeFileSystem()
	paths := SummonPaths{
		BinaryPath: "/usr/local/bin/summon",
		BinaryDir:  "/usr/local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	err := UninstallWith(paths, &buf, fs)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "system-managed location")
	assert.Contains(t, buf.String(), "package manager")
}

func TestUninstallWith_CorrectRemovalOrder(t *testing.T) {
	// Verify config dir is removed before binary
	var order []string
	fs := &orderTrackingFS{order: &order}
	fs.statPaths = map[string]bool{"/home/user/.summon": true}
	paths := SummonPaths{
		BinaryPath: "/home/user/.local/bin/summon",
		BinaryDir:  "/home/user/.local/bin",
		ConfigDir:  "/home/user/.summon",
	}
	var buf bytes.Buffer

	err := UninstallWith(paths, &buf, fs)
	require.NoError(t, err)
	require.Len(t, order, 2)
	assert.Equal(t, "removeall:/home/user/.summon", order[0])
	assert.Equal(t, "remove:/home/user/.local/bin/summon", order[1])
}

type orderTrackingFS struct {
	fakeFileSystem
	order *[]string
}

func (f *orderTrackingFS) RemoveAll(path string) error {
	*f.order = append(*f.order, "removeall:"+path)
	return nil
}

func (f *orderTrackingFS) Remove(path string) error {
	*f.order = append(*f.order, "remove:"+path)
	return nil
}

func TestRemoveBinary_NonPermissionError(t *testing.T) {
	fs := newFakeFileSystem()
	fs.removeErr["/home/user/.local/bin/summon"] = fmt.Errorf("device not ready")
	var buf bytes.Buffer

	err := removeBinary("/home/user/.local/bin/summon", fs, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove")
	assert.Contains(t, err.Error(), "device not ready")
}

func TestIsSystemWidePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/usr/local/bin/summon", true},
		{"/usr/bin/summon", true},
		{"/opt/summon/bin/summon", true},
		{"/home/user/.local/bin/summon", false},
		{"/Users/user/.local/bin/summon", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSystemWidePath(tt.path))
		})
	}
}
