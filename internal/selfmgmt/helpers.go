package selfmgmt

import (
	"os"
	"path/filepath"
)

// createTempScript writes script content to a temporary file and returns the path.
func createTempScript(content []byte, name string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "summon-update-*")
	if err != nil {
		return "", err
	}
	tmpFile := filepath.Join(tmpDir, name)
	if err := os.WriteFile(tmpFile, content, 0700); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpFile, nil
}

// removeTempFile removes a temporary script file and its parent directory.
func removeTempFile(path string) {
	_ = os.RemoveAll(filepath.Dir(path))
}

// buildInstallerEnv constructs the environment variables for the installer subprocess.
// It inherits the current environment and adds/overrides summon-specific variables.
func buildInstallerEnv(installDir, versionTag string) []string {
	env := os.Environ()
	env = append(env,
		"SUMMON_INSTALL_DIR="+installDir,
		"SUMMON_VERSION="+versionTag,
		"SUMMON_NO_MODIFY_PATH=1",
	)
	return env
}
