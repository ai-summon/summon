package selfmgmt

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePathResolver struct {
	executablePath string
	executableErr  error
	symlinksMap    map[string]string
	symlinkErr     error
	homeDir        string
	homeDirErr     error
}

func (f *fakePathResolver) Executable() (string, error) {
	return f.executablePath, f.executableErr
}

func (f *fakePathResolver) EvalSymlinks(path string) (string, error) {
	if f.symlinkErr != nil {
		return "", f.symlinkErr
	}
	if resolved, ok := f.symlinksMap[path]; ok {
		return resolved, nil
	}
	return path, nil
}

func (f *fakePathResolver) UserHomeDir() (string, error) {
	return f.homeDir, f.homeDirErr
}

func TestResolvePathsWith_HappyPath(t *testing.T) {
	resolver := &fakePathResolver{
		executablePath: "/usr/local/bin/summon",
		homeDir:        "/home/testuser",
	}

	paths, err := ResolvePathsWith(resolver)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/summon", paths.BinaryPath)
	assert.Equal(t, "/usr/local/bin", paths.BinaryDir)
	assert.Equal(t, "/home/testuser/.summon", paths.ConfigDir)
}

func TestResolvePathsWith_SymlinkResolution(t *testing.T) {
	resolver := &fakePathResolver{
		executablePath: "/usr/local/bin/summon",
		symlinksMap: map[string]string{
			"/usr/local/bin/summon": "/opt/summon/bin/summon",
		},
		homeDir: "/home/testuser",
	}

	paths, err := ResolvePathsWith(resolver)
	require.NoError(t, err)
	assert.Equal(t, "/opt/summon/bin/summon", paths.BinaryPath)
	assert.Equal(t, "/opt/summon/bin", paths.BinaryDir)
}

func TestResolvePathsWith_ExecutableError(t *testing.T) {
	resolver := &fakePathResolver{
		executableErr: fmt.Errorf("not available"),
	}

	_, err := ResolvePathsWith(resolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine binary path")
}

func TestResolvePathsWith_SymlinkError(t *testing.T) {
	resolver := &fakePathResolver{
		executablePath: "/usr/local/bin/summon",
		symlinkErr:     fmt.Errorf("broken symlink"),
	}

	_, err := ResolvePathsWith(resolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve symlinks")
}

func TestResolvePathsWith_HomeDirError(t *testing.T) {
	resolver := &fakePathResolver{
		executablePath: "/usr/local/bin/summon",
		homeDirErr:     fmt.Errorf("homeless"),
	}

	_, err := ResolvePathsWith(resolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine home directory")
}
