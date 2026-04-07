package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CreateDirLink
// ---------------------------------------------------------------------------

func TestCreateDirLink(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(source, 0o755))

	target := filepath.Join(dir, "link")
	require.NoError(t, CreateDirLink(source, target))

	// The target should be a link (symlink on Unix, junction on Windows).
	assert.True(t, IsLink(target), "target should be a link")

	dest, err := os.Readlink(target)
	require.NoError(t, err)
	assert.Equal(t, source, dest)
}

func TestCreateDirLink_TargetAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(source, 0o755))

	target := filepath.Join(dir, "link")
	require.NoError(t, CreateDirLink(source, target))

	// Creating a second link at the same target should fail.
	err := CreateDirLink(source, target)
	assert.Error(t, err)
}

func TestCreateDirLink_SourceDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "nonexistent")
	target := filepath.Join(dir, "link")

	// Symlink creation succeeds even if the source doesn't exist (dangling link).
	err := CreateDirLink(source, target)
	require.NoError(t, err)
	assert.True(t, IsLink(target), "dangling symlink should still be a link")
}

func TestCreateDirLink_ContentAccessible(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "hello.txt"), []byte("world"), 0o644))

	target := filepath.Join(dir, "link")
	require.NoError(t, CreateDirLink(source, target))

	// Files in source should be accessible through the link.
	data, err := os.ReadFile(filepath.Join(target, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "world", string(data))
}

// ---------------------------------------------------------------------------
// RemoveLink
// ---------------------------------------------------------------------------

func TestRemoveLink(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(source, 0o755))

	link := filepath.Join(dir, "link")
	require.NoError(t, CreateDirLink(source, link))

	require.NoError(t, RemoveLink(link))

	// Link should be gone.
	_, err := os.Lstat(link)
	assert.True(t, os.IsNotExist(err))

	// Original source directory should still exist.
	_, err = os.Stat(source)
	assert.NoError(t, err, "source should not be deleted when link is removed")
}

func TestRemoveLink_NonexistentPath(t *testing.T) {
	err := RemoveLink(filepath.Join(t.TempDir(), "nope"))
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// IsLink
// ---------------------------------------------------------------------------

func TestIsLink_Symlink(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(source, 0o755))

	link := filepath.Join(dir, "link")
	require.NoError(t, CreateDirLink(source, link))

	assert.True(t, IsLink(link))
}

func TestIsLink_RegularDir(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsLink(dir), "a regular directory is not a link")
}

func TestIsLink_RegularFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(f, []byte("data"), 0o644))
	assert.False(t, IsLink(f), "a regular file is not a link")
}

func TestIsLink_NonexistentPath(t *testing.T) {
	assert.False(t, IsLink(filepath.Join(t.TempDir(), "ghost")))
}

func TestIsLink_DanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	require.NoError(t, os.Symlink("/nonexistent/target", link))

	assert.True(t, IsLink(link), "a dangling symlink is still a link")
}

// ---------------------------------------------------------------------------
// LinkTarget
// ---------------------------------------------------------------------------

func TestLinkTarget(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(source, 0o755))

	link := filepath.Join(dir, "link")
	require.NoError(t, CreateDirLink(source, link))

	target, err := LinkTarget(link)
	require.NoError(t, err)
	assert.Equal(t, source, target)
}

func TestLinkTarget_NonLink(t *testing.T) {
	_, err := LinkTarget(t.TempDir())
	assert.Error(t, err, "reading link target of a non-link should fail")
}

func TestLinkTarget_NonexistentPath(t *testing.T) {
	_, err := LinkTarget(filepath.Join(t.TempDir(), "nope"))
	assert.Error(t, err)
}

func TestLinkTarget_DanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	expectedTarget := "/nonexistent/target"
	require.NoError(t, os.Symlink(expectedTarget, link))

	target, err := LinkTarget(link)
	require.NoError(t, err)
	assert.Equal(t, expectedTarget, target, "should return the target even if it doesn't exist")
}
