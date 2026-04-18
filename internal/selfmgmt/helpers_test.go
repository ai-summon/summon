package selfmgmt

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTempScript_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not applicable on Windows")
	}
	path, err := createTempScript([]byte("#!/bin/sh\necho hello"), "test.sh")
	require.NoError(t, err)
	defer removeTempFile(path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho hello", string(content))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestCreateTempScript_MkdirTempError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TMPDIR env var not used on Windows")
	}
	t.Setenv("TMPDIR", "/nonexistent/path/for/testing")

	_, err := createTempScript([]byte("test"), "test.sh")
	require.Error(t, err)
}

func TestBuildInstallerEnv(t *testing.T) {
	env := buildInstallerEnv("/opt/summon/bin", "v0.2.0")

	found := map[string]bool{}
	for _, e := range env {
		switch e {
		case "SUMMON_INSTALL_DIR=/opt/summon/bin":
			found["install_dir"] = true
		case "SUMMON_VERSION=v0.2.0":
			found["version"] = true
		case "SUMMON_NO_MODIFY_PATH=1":
			found["no_modify"] = true
		}
	}
	assert.True(t, found["install_dir"], "should set SUMMON_INSTALL_DIR")
	assert.True(t, found["version"], "should set SUMMON_VERSION")
	assert.True(t, found["no_modify"], "should set SUMMON_NO_MODIFY_PATH")
}
