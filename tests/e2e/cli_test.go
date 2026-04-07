package e2e

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	name := "summon"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", binary, "../../cmd/summon")
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err, "failed to build summon binary")
	return binary
}

func TestInit(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	cmd := exec.Command(binary, "init", "--name", "test-pkg")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Initialized package")
	_, err = os.Stat(filepath.Join(dir, "summon.yaml"))
	assert.NoError(t, err)
	for _, d := range []string{"skills", "agents", "commands"} {
		_, err = os.Stat(filepath.Join(dir, d))
		assert.NoError(t, err, "expected directory: "+d)
	}
}

func TestInitDuplicate(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	cmd := exec.Command(binary, "init", "--name", "test-pkg")
	cmd.Dir = dir
	_, err := cmd.CombinedOutput()
	require.NoError(t, err)
	cmd = exec.Command(binary, "init", "--name", "test-pkg")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "already exists")
}

func TestInstallLocal(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "my-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	manifest := "name: my-pkg\nversion: \"1.0.0\"\ndescription: \"Test package\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "summon.yaml"), []byte(manifest), 0o644))
	cmd := exec.Command(binary, "install", "--path", pkgDir, "--force")
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(), "SUMMON_VSCODE_SETTINGS_DIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Installed my-pkg v1.0.0")
}

func TestListEmpty(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	cmd := exec.Command(binary, "list", "--scope", "local")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "No packages installed")
}

func TestListJSON(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "json-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	manifest := "name: json-pkg\nversion: \"1.0.0\"\ndescription: \"Test\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "summon.yaml"), []byte(manifest), 0o644))
	vscodeDir := t.TempDir()
	installCmd := exec.Command(binary, "install", "--path", pkgDir, "--force")
	installCmd.Dir = projectDir
	installCmd.Env = append(os.Environ(), "SUMMON_VSCODE_SETTINGS_DIR="+vscodeDir)
	_, err := installCmd.CombinedOutput()
	require.NoError(t, err)
	cmd := exec.Command(binary, "list", "--json")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.True(t, strings.HasPrefix(strings.TrimSpace(string(out)), "["))
	assert.Contains(t, string(out), "json-pkg")
}

func TestUninstall(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	pkgDir := filepath.Join(t.TempDir(), "rm-pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	manifest := "name: rm-pkg\nversion: \"1.0.0\"\ndescription: \"Test\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "summon.yaml"), []byte(manifest), 0o644))
	vscodeDir := t.TempDir()
	envWithOverride := append(os.Environ(), "SUMMON_VSCODE_SETTINGS_DIR="+vscodeDir)
	installCmd := exec.Command(binary, "install", "--path", pkgDir, "--force")
	installCmd.Dir = projectDir
	installCmd.Env = envWithOverride
	_, err := installCmd.CombinedOutput()
	require.NoError(t, err)
	cmd := exec.Command(binary, "uninstall", "rm-pkg")
	cmd.Dir = projectDir
	cmd.Env = envWithOverride
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Uninstalled rm-pkg")
	listCmd := exec.Command(binary, "list", "--scope", "local")
	listCmd.Dir = projectDir
	listOut, err := listCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(listOut), "No packages installed")
}

func TestVersion(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary, "--version")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "0.1.0")
}

// ---------------------------------------------------------------------------
// T002: Scoped install fixture scaffolding helpers
// ---------------------------------------------------------------------------

// installScopedPkg installs a minimal local package into the given project
// directory using the specified scope. Returns the package name and the
// absolute path to the package directory.
func installScopedPkg(t *testing.T, binary, projectDir, pkgName, scope string, env []string) {
	t.Helper()
	pkgDir := filepath.Join(t.TempDir(), pkgName)
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	manifest := "name: " + pkgName + "\nversion: \"1.0.0\"\ndescription: \"e2e scope test\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "summon.yaml"), []byte(manifest), 0o644))

	args := []string{"install", "--path", pkgDir, "--force"}
	if scope != "" {
		args = append(args, "--scope", scope)
	}
	cmd := exec.Command(binary, args...)
	cmd.Dir = projectDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install %s --scope %s: %s", pkgName, scope, out)
}

// makeEnv returns a copy of the process environment with the given key=value
// overrides appended.
func makeEnv(overrides ...string) []string {
	return append(os.Environ(), overrides...)
}

// ---------------------------------------------------------------------------
// T010: Default-scope restore (project only, not local)
// ---------------------------------------------------------------------------

// TestInstall_NoArgs_RestoresProjectScope verifies that `summon install`
// (no package argument) reads and restores only the project scope registry.
func TestInstall_NoArgs_RestoresProjectScope(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	vscodeDir := t.TempDir()
	env := makeEnv("SUMMON_VSCODE_SETTINGS_DIR=" + vscodeDir)

	// Install one package at project scope and one at local scope.
	installScopedPkg(t, binary, projectDir, "proj-pkg", "project", env)
	installScopedPkg(t, binary, projectDir, "local-pkg", "local", env)

	// Simulate a fresh clone: remove both store directories.
	require.NoError(t, os.RemoveAll(filepath.Join(projectDir, ".summon", "project", "store")))
	require.NoError(t, os.RemoveAll(filepath.Join(projectDir, ".summon", "local", "store")))

	// `summon install` with no args restores project scope by default.
	restoreCmd := exec.Command(binary, "install")
	restoreCmd.Dir = projectDir
	restoreCmd.Env = env
	out, err := restoreCmd.CombinedOutput()
	require.NoError(t, err, string(out))

	// project scope package should be restored (or skipped if already present).
	outStr := string(out)
	assert.True(t,
		strings.Contains(outStr, "proj-pkg") || strings.Contains(outStr, "All packages restored"),
		"project-scope package should be mentioned in restore output: %s", outStr)

	// local scope package is NOT restored by the default `summon install`.
	assert.NotContains(t, outStr, "local-pkg",
		"local-scope package should NOT be restored by default summon install")
}

// TestInstall_NoArgs_LocalScopeReturnsError verifies that `summon install`
// with `--scope local` returns an error (local scope cannot be restored from
// shared state).
func TestInstall_NoArgs_ScopeFlagLocalReturnsError(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	cmd := exec.Command(binary, "install", "--scope", "local")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err, "install --scope local should fail when no args given")
	assert.Contains(t, string(out), "local", "error should mention local scope")
}

// ---------------------------------------------------------------------------
// T017: Copilot workspace-leak regression scenarios
// ---------------------------------------------------------------------------

// TestInstall_ProjectScope_DoesNotLeakToUserSettings verifies that a
// project-scope install does NOT write to VS Code user-level settings.
// The SUMMON_VSCODE_SETTINGS_DIR override allows us to inspect what would
// have been written to the real user settings directory.
func TestInstall_ProjectScope_DoesNotLeakToUserSettings(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	vscodeDir := t.TempDir()
	env := makeEnv("SUMMON_VSCODE_SETTINGS_DIR=" + vscodeDir)

	installScopedPkg(t, binary, projectDir, "proj-only-pkg", "project", env)

	// The user-level settings file should either not exist or not contain the
	// project-scope package's chat.pluginLocations entry.
	userSettingsPath := filepath.Join(vscodeDir, "settings.json")
	if data, err := os.ReadFile(userSettingsPath); err == nil {
		settingsStr := string(data)
		// chat.pluginLocations in user settings would mean the project-scope
		// install leaked to user-global settings.
		// The store path for project scope is .summon/project/store/
		assert.NotContains(t, settingsStr,
			filepath.Join(projectDir, ".summon", "project", "store", "proj-only-pkg"),
			"project-scope package must not appear in user-level chat.pluginLocations")
	}
	// If the file doesn't exist, that's also acceptable: it means nothing was
	// written to user settings, which is the desired behavior.
}

// TestInstall_LocalScope_WritesWorkspaceAndUserSettings verifies that a
// local-scope install DOES write to VS Code user-level settings (since VS Code
// chat.pluginLocations is application-scoped and must be in user settings for
// activation).
func TestInstall_LocalScope_WritesUserSettings(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()
	vscodeDir := t.TempDir()
	homeDir := t.TempDir()
	appDataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "Code", "User"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, "Library", "Application Support", "Code", "User"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(appDataDir, "Code", "User"), 0o755))
	env := makeEnv(
		"SUMMON_VSCODE_SETTINGS_DIR="+vscodeDir,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"APPDATA="+appDataDir,
	)

	installScopedPkg(t, binary, projectDir, "local-act-pkg", "local", env)

	// User-level settings SHOULD contain the local-scope package path so that
	// VS Code can activate it.
	userSettingsPath := filepath.Join(vscodeDir, "settings.json")
	data, err := os.ReadFile(userSettingsPath)
	require.NoError(t, err, "user settings should be written for local scope install")
	assert.Contains(t, string(data), "local-act-pkg",
		"local-scope package should appear in user-level settings for VS Code activation")
}

func writeExecutableFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

func installerScriptPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(wd, "..", "..", "scripts", "install.sh"))
}

func windowsInstallerScriptPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(wd, "..", "..", "scripts", "install.ps1"))
}

func buildFileURI(absPath string) string {
	return "file://" + filepath.ToSlash(absPath)
}

func firstAvailableCommand(names ...string) string {
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

func TestInstallerScript_UnixHappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source")
	writeExecutableFile(t, sourceBin, "#!/usr/bin/env sh\necho 9.9.9\n")

	data, err := os.ReadFile(sourceBin)
	require.NoError(t, err)
	checksum := sha256Hex(data)

	targetPath := filepath.Join(projectDir, "bin", "summon")
	cmd := exec.Command("sh", installerScriptPath(t))
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+targetPath,
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Installed summon at:")

	verify := exec.Command(targetPath, "--version")
	verifyOut, err := verify.CombinedOutput()
	require.NoError(t, err, string(verifyOut))
	assert.Contains(t, string(verifyOut), "9.9.9")
}

func TestInstallerScript_ChecksumMismatchFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source")
	writeExecutableFile(t, sourceBin, "#!/usr/bin/env sh\necho 9.9.9\n")

	targetPath := filepath.Join(projectDir, "bin", "summon")
	cmd := exec.Command("sh", installerScriptPath(t))
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM=deadbeef",
		"SUMMON_INSTALL_PATH="+targetPath,
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "installer should fail on checksum mismatch")
	assert.Contains(t, string(out), "ERROR[checksum]")
}

func TestInstallerScript_UnsupportedPlatformFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	cmd := exec.Command("sh", installerScriptPath(t))
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"SUMMON_TEST_OS=plan9",
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(out), "ERROR[platform]")
}

func TestInstallerScript_RerunIsIdempotentForTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source")
	writeExecutableFile(t, sourceBin, "#!/usr/bin/env sh\necho 9.9.9\n")

	data, err := os.ReadFile(sourceBin)
	require.NoError(t, err)
	checksum := sha256Hex(data)

	targetPath := filepath.Join(projectDir, "bin", "summon")
	baseEnv := append(os.Environ(),
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+targetPath,
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)

	for i := 0; i < 2; i++ {
		cmd := exec.Command("sh", installerScriptPath(t))
		cmd.Dir = projectDir
		cmd.Env = baseEnv
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	entries, err := os.ReadDir(filepath.Dir(targetPath))
	require.NoError(t, err)
	count := 0
	for _, e := range entries {
		if e.Name() == "summon" {
			count++
		}
	}
	assert.Equal(t, 1, count, "rerun should keep a single active target binary")
}

func TestInstallerScript_WindowsHappyPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows installer happy path runs on windows only")
	}

	pwsh := firstAvailableCommand("pwsh", "powershell")
	if pwsh == "" {
		t.Skip("PowerShell is not available")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source.exe")
	buildSource := exec.Command("go", "build", "-o", sourceBin, "../../cmd/summon")
	wd, err := os.Getwd()
	require.NoError(t, err)
	buildSource.Dir = wd
	buildOut, err := buildSource.CombinedOutput()
	require.NoError(t, err, string(buildOut))

	data, err := os.ReadFile(sourceBin)
	require.NoError(t, err)
	checksum := sha256Hex(data)

	targetPath := filepath.Join(projectDir, "bin", "summon.exe")
	cmd := exec.Command(pwsh, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", windowsInstallerScriptPath(t))
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL="+buildFileURI(sourceBin),
		"SUMMON_TEST_ARCH=amd64",
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+targetPath,
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Installed summon at:")

	verify := exec.Command(targetPath, "--version")
	verifyOut, err := verify.CombinedOutput()
	require.NoError(t, err, string(verifyOut))
	assert.Contains(t, string(verifyOut), "summon version")
}

func TestInstallerScript_CINonInteractiveProducesUsableBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source")
	writeExecutableFile(t, sourceBin, "#!/usr/bin/env sh\necho 9.9.9\n")

	data, err := os.ReadFile(sourceBin)
	require.NoError(t, err)
	checksum := sha256Hex(data)

	targetPath := filepath.Join(projectDir, "bin", "summon")
	cmd := exec.Command("sh", installerScriptPath(t))
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"CI=1",
		"SUMMON_NONINTERACTIVE=1",
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+targetPath,
		"SUMMON_NO_MODIFY_PATH=1",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	verify := exec.Command(targetPath, "--version")
	verifyOut, err := verify.CombinedOutput()
	require.NoError(t, err, string(verifyOut))
	assert.Contains(t, string(verifyOut), "9.9.9")
}

func TestInstallerScript_PathOptOutAndFallbackMessages(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source")
	writeExecutableFile(t, sourceBin, "#!/usr/bin/env sh\necho 9.9.9\n")
	data, err := os.ReadFile(sourceBin)
	require.NoError(t, err)
	checksum := sha256Hex(data)

	optOutTarget := filepath.Join(projectDir, "optout", "summon")
	optOut := exec.Command("sh", installerScriptPath(t))
	optOut.Dir = projectDir
	optOut.Env = append(os.Environ(),
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+optOutTarget,
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)
	optOutOutput, err := optOut.CombinedOutput()
	require.NoError(t, err, string(optOutOutput))
	assert.Contains(t, string(optOutOutput), "PATH update skipped")

	fallbackTarget := filepath.Join(projectDir, "fallback", "summon")
	bogusHome := filepath.Join(projectDir, "missing-home")
	fallback := exec.Command("sh", installerScriptPath(t))
	fallback.Dir = projectDir
	fallback.Env = append(os.Environ(),
		"HOME="+bogusHome,
		"SHELL=/bin/bash",
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+fallbackTarget,
		"SUMMON_NONINTERACTIVE=1",
	)
	fallbackOutput, err := fallback.CombinedOutput()
	require.NoError(t, err, string(fallbackOutput))
	assert.Contains(t, string(fallbackOutput), "Run manually: export PATH=")
}

func TestInstallerScript_ShadowedBinaryWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer test only runs on unix-like systems")
	}

	projectDir := t.TempDir()
	sourceBin := filepath.Join(t.TempDir(), "summon-source")
	writeExecutableFile(t, sourceBin, "#!/usr/bin/env sh\necho 9.9.9\n")
	data, err := os.ReadFile(sourceBin)
	require.NoError(t, err)
	checksum := sha256Hex(data)

	shadowDir := filepath.Join(projectDir, "shadow")
	require.NoError(t, os.MkdirAll(shadowDir, 0o755))
	shadowBinary := filepath.Join(shadowDir, "summon")
	writeExecutableFile(t, shadowBinary, "#!/usr/bin/env sh\necho shadow\n")

	targetPath := filepath.Join(projectDir, "bin", "summon")
	cmd := exec.Command("sh", installerScriptPath(t))
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"PATH="+shadowDir+":"+os.Getenv("PATH"),
		"SUMMON_TEST_ALLOW_INSECURE_URLS=1",
		"SUMMON_DOWNLOAD_URL=file://"+sourceBin,
		"SUMMON_CHECKSUM="+checksum,
		"SUMMON_INSTALL_PATH="+targetPath,
		"SUMMON_NO_MODIFY_PATH=1",
		"SUMMON_NONINTERACTIVE=1",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Another summon binary appears earlier in PATH")
}

// ---------------------------------------------------------------------------
// plugin.json fallback E2E (004-plugin-json-fallback)
// ---------------------------------------------------------------------------

func TestPluginJSONFallback_E2E(t *testing.T) {
	binary := buildBinary(t)
	projectDir := t.TempDir()

	// Create a local package with only .claude-plugin/plugin.json
	pkgDir := filepath.Join(t.TempDir(), "pj-e2e-pkg")
	claudeDir := filepath.Join(pkgDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "skills"), 0o755))
	pj := `{"name":"pj-e2e-pkg","version":"1.0.0","description":"E2E plugin.json test"}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "plugin.json"), []byte(pj), 0o644))

	// Install
	cmd := exec.Command(binary, "install", "--path", pkgDir, "--scope", "local", "--force")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install failed: "+string(out))
	assert.Contains(t, string(out), "Installed pj-e2e-pkg v1.0.0")

	// List
	cmd = exec.Command(binary, "list", "--scope", "local")
	cmd.Dir = projectDir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "list failed: "+string(out))
	assert.Contains(t, string(out), "pj-e2e-pkg")

	// Uninstall
	cmd = exec.Command(binary, "uninstall", "pj-e2e-pkg", "--scope", "local")
	cmd.Dir = projectDir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "uninstall failed: "+string(out))

	// Verify gone from list
	cmd = exec.Command(binary, "list", "--scope", "local")
	cmd.Dir = projectDir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "list after uninstall failed: "+string(out))
	assert.NotContains(t, string(out), "pj-e2e-pkg")
}
