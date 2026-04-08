package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveDir_RenameSuccess(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "pkg")
	target := filepath.Join(targetRoot, "pkg")

	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("hello"), 0o644))

	require.NoError(t, MoveDir(source, target))

	_, err := os.Stat(filepath.Join(target, "a.txt"))
	require.NoError(t, err)
	_, err = os.Stat(source)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveDir_CrossDeviceFallback(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "pkg")
	target := filepath.Join(targetRoot, "pkg")

	require.NoError(t, os.MkdirAll(filepath.Join(source, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "nested", "a.txt"), []byte("hello"), 0o644))

	origRename := renameDir
	renameDir = func(_, _ string) error {
		return &os.LinkError{Op: "rename", Old: source, New: target, Err: syscall.EXDEV}
	}
	t.Cleanup(func() { renameDir = origRename })

	require.NoError(t, MoveDir(source, target))

	data, err := os.ReadFile(filepath.Join(target, "nested", "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
	_, err = os.Stat(source)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveDir_NonCrossDeviceError(t *testing.T) {
	source := filepath.Join(t.TempDir(), "pkg")
	target := filepath.Join(t.TempDir(), "pkg")
	require.NoError(t, os.MkdirAll(source, 0o755))

	origRename := renameDir
	renameDir = func(_, _ string) error {
		return errors.New("permission denied")
	}
	t.Cleanup(func() { renameDir = origRename })

	err := MoveDir(source, target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}
