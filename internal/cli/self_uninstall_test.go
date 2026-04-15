package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSelfUninstallFS struct {
	removedAll []string
	removed    []string
	statPaths  map[string]bool
	removeErr  map[string]error
}

func newFakeSelfUninstallFS() *fakeSelfUninstallFS {
	return &fakeSelfUninstallFS{
		statPaths: make(map[string]bool),
		removeErr: make(map[string]error),
	}
}

func (f *fakeSelfUninstallFS) RemoveAll(path string) error {
	if err, ok := f.removeErr[path]; ok {
		return err
	}
	f.removedAll = append(f.removedAll, path)
	return nil
}

func (f *fakeSelfUninstallFS) Remove(path string) error {
	if err, ok := f.removeErr[path]; ok {
		return err
	}
	f.removed = append(f.removed, path)
	return nil
}

func (f *fakeSelfUninstallFS) Stat(path string) (os.FileInfo, error) {
	if exists, ok := f.statPaths[path]; ok && exists {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

func TestSelfUninstall_ConfirmedRuns(t *testing.T) {
	// Reset flag for test isolation
	selfUninstallConfirm = false

	fs := newFakeSelfUninstallFS()
	fs.statPaths["/home/user/.summon"] = true
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader("y\n"),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "summon is now uninstalled")
	assert.Contains(t, stdout.String(), "plugins installed in native CLI platforms")
}

func TestSelfUninstall_DeclinedExitsCleanly(t *testing.T) {
	selfUninstallConfirm = false

	fs := newFakeSelfUninstallFS()
	fs.statPaths["/home/user/.summon"] = true
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader("n\n"),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), "summon is now uninstalled")
	assert.Empty(t, fs.removed, "nothing should be removed on decline")
}

func TestSelfUninstall_PathsDisplayedBeforePrompt(t *testing.T) {
	selfUninstallConfirm = false

	fs := newFakeSelfUninstallFS()
	fs.statPaths["/home/user/.summon"] = true
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader("n\n"),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)
	output := stdout.String()
	assert.Contains(t, output, "This will remove:")
	assert.Contains(t, output, "/home/user/.local/bin/summon")
	assert.Contains(t, output, "/home/user/.summon")
}

func TestSelfUninstall_ConfirmFlagSkipsPrompt(t *testing.T) {
	selfUninstallConfirm = true
	defer func() { selfUninstallConfirm = false }()

	fs := newFakeSelfUninstallFS()
	fs.statPaths["/home/user/.summon"] = true
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader(""), // no input — should not block
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "summon is now uninstalled")
	assert.NotContains(t, stdout.String(), "Remove summon and all configuration data?")
}

func TestSelfUninstall_ConfigDirMissingNotDisplayed(t *testing.T) {
	selfUninstallConfirm = true
	defer func() { selfUninstallConfirm = false }()

	fs := newFakeSelfUninstallFS()
	// Config dir does NOT exist
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader(""),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)
	output := stdout.String()
	assert.Contains(t, output, "This will remove:")
	assert.Contains(t, output, "/home/user/.local/bin/summon")
	// Config dir path should NOT be displayed when it doesn't exist
	lines := strings.Split(output, "\n")
	configShown := false
	for _, line := range lines {
		if strings.Contains(line, ".summon/") && strings.HasPrefix(strings.TrimSpace(line), "/") {
			configShown = true
		}
	}
	assert.False(t, configShown, "config dir should not be displayed when missing")
}

func TestSelfUninstall_PluginNoteDisplayed(t *testing.T) {
	selfUninstallConfirm = true
	defer func() { selfUninstallConfirm = false }()

	fs := newFakeSelfUninstallFS()
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/home/user/.local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader(""),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "plugins installed in native CLI platforms")
}

// fakePathResolver is a test double for selfmgmt.PathResolver.
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
	if f.symlinksMap != nil {
		if resolved, ok := f.symlinksMap[path]; ok {
			return resolved, nil
		}
	}
	return path, nil
}

func (f *fakePathResolver) UserHomeDir() (string, error) {
	return f.homeDir, f.homeDirErr
}

func TestSelfUninstall_PathResolveError(t *testing.T) {
	selfUninstallConfirm = true
	defer func() { selfUninstallConfirm = false }()

	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executableErr: fmt.Errorf("cannot determine binary"),
		},
		stdin:  strings.NewReader(""),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.Error(t, err)
}
