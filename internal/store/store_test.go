package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	s := New("/tmp/test-store")
	assert.Equal(t, "/tmp/test-store", s.Dir)
}

func TestInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	s := New(dir)
	err := s.Init()
	require.NoError(t, err)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestPackagePath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "tmp", "store")
	s := New(base)
	assert.Equal(t, filepath.Join(base, "my-pkg"), s.PackagePath("my-pkg"))
}

func TestHas_Missing(t *testing.T) {
	s := New(t.TempDir())
	assert.False(t, s.Has("nonexistent"))
}

func TestLink_And_Has(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	sourceDir := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	s := New(storeDir)
	err := s.Link("my-pkg", sourceDir)
	require.NoError(t, err)
	assert.True(t, s.Has("my-pkg"))
}

func TestRemoveFromStore(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	sourceDir := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	s := New(storeDir)
	require.NoError(t, s.Link("my-pkg", sourceDir))
	assert.True(t, s.Has("my-pkg"))
	err := s.Remove("my-pkg")
	require.NoError(t, err)
	assert.False(t, s.Has("my-pkg"))
}

func TestRemove_NotFound(t *testing.T) {
	s := New(t.TempDir())
	err := s.Remove("nonexistent")
	assert.NoError(t, err)
}

func TestList(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o755))
	s := New(storeDir)
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "pkg-a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "pkg-b"), 0o755))
	names, err := s.List()
	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "pkg-a")
	assert.Contains(t, names, "pkg-b")
}

func TestList_Empty(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "nonexistent"))
	names, err := s.List()
	require.NoError(t, err)
	assert.Nil(t, names)
}

func TestIsBrokenLink(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	sourceDir := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	s := New(storeDir)
	require.NoError(t, s.Link("my-pkg", sourceDir))
	assert.False(t, s.IsBrokenLink("my-pkg"))
	require.NoError(t, os.RemoveAll(sourceDir))
	assert.True(t, s.IsBrokenLink("my-pkg"))
}

func TestRemove_RegularDirectory(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o755))
	s := New(storeDir)
	pkgDir := filepath.Join(storeDir, "real-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "file.txt"), []byte("data"), 0o644))
	assert.True(t, s.Has("real-pkg"))
	err := s.Remove("real-pkg")
	require.NoError(t, err)
	assert.False(t, s.Has("real-pkg"))
}

func TestLink_ReplacesExisting(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	source1 := filepath.Join(t.TempDir(), "src1")
	source2 := filepath.Join(t.TempDir(), "src2")
	require.NoError(t, os.MkdirAll(source1, 0o755))
	require.NoError(t, os.MkdirAll(source2, 0o755))

	s := New(storeDir)
	require.NoError(t, s.Link("pkg", source1))

	target1, err := os.Readlink(s.PackagePath("pkg"))
	require.NoError(t, err)

	require.NoError(t, s.Link("pkg", source2))

	target2, err := os.Readlink(s.PackagePath("pkg"))
	require.NoError(t, err)
	assert.NotEqual(t, target1, target2)

	absSource2, _ := filepath.Abs(source2)
	assert.Equal(t, absSource2, target2)
}

func TestIsBrokenLink_RegularDir(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "real-dir"), 0o755))
	s := New(storeDir)
	assert.False(t, s.IsBrokenLink("real-dir"))
}

func TestIsBrokenLink_NonExistent(t *testing.T) {
	s := New(t.TempDir())
	assert.False(t, s.IsBrokenLink("does-not-exist"))
}

func TestInit_Idempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	s := New(dir)
	require.NoError(t, s.Init())
	require.NoError(t, s.Init())
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestList_WithMixedEntries(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o755))
	s := New(storeDir)

	// Create a regular directory
	require.NoError(t, os.MkdirAll(filepath.Join(storeDir, "dir-pkg"), 0o755))

	// Create a symlink
	sourceDir := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, s.Link("link-pkg", sourceDir))

	names, err := s.List()
	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "dir-pkg")
	assert.Contains(t, names, "link-pkg")
}
