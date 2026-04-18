package cli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update golden test baselines")

// testdataDir returns the absolute path to the testdata directory.
// Computed once at test init time, before any os.Chdir calls.
var testdataDir string

func init() {
	// Resolve absolute path to testdata/ at init time
	abs, err := filepath.Abs("testdata")
	if err != nil {
		panic("cannot resolve testdata dir: " + err.Error())
	}
	testdataDir = abs
}

// assertGoldenString compares got against a .golden file in testdata/.
// If -update flag is set, writes got to the golden file instead.
func assertGoldenString(t *testing.T, got, filename string) {
	t.Helper()
	goldenPath := filepath.Join(testdataDir, filename)

	if *updateGolden {
		err := os.MkdirAll(testdataDir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(goldenPath, []byte(got), 0644)
		require.NoError(t, err)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file %s not found; run with -update to create it", goldenPath)
	}
	require.NoError(t, err)

	// Normalize for cross-platform comparison:
	// - CRLF → LF (git autocrlf on Windows)
	// - Backslash → forward slash (Windows path separators)
	normalizeGolden := func(s string) string {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\\", "/")
		return s
	}

	normalizedExpected := normalizeGolden(string(expected))
	normalizedGot := normalizeGolden(got)

	if normalizedGot != normalizedExpected {
		t.Errorf("golden mismatch for %s:\n--- expected ---\n%s\n--- got ---\n%s",
			filename, normalizedExpected, normalizedGot)
	}
}

// ======== Golden Tests: Install ========

func TestGolden_Install_Success(t *testing.T) {
	runner := newFakeRunner()
	fetcher := newFakeFetcher()
	adapter := newFakeAdapter("claude")

	deps := newTestDeps(runner, fetcher, []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assertGoldenString(t, out, "install-success.golden")
}

func TestGolden_Install_Error(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.installFunc = func(source string, scope platform.Scope) error {
		return fmt.Errorf("network timeout")
	}

	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("gh:owner/my-plugin", deps)
	// Install reports partial success/failure, may or may not return error
	_ = err

	out := deps.stdout.(*bytes.Buffer).String()
	assertGoldenString(t, out, "install-error.golden")
}

// ======== Golden Tests: Uninstall ========

func TestGolden_Uninstall_Success(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	err := runUninstall("my-plugin", deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assertGoldenString(t, out, "uninstall-success.golden")
}

func TestGolden_Uninstall_Error(t *testing.T) {
	adapter := newFakeAdapter("claude")
	adapter.listInstalledFunc = func(scope platform.Scope) ([]platform.InstalledPlugin, error) {
		return []platform.InstalledPlugin{
			{Name: "my-plugin", Source: "my-plugin@marketplace", Platform: "claude"},
		}, nil
	}
	adapter.uninstallFunc = func(name string, scope platform.Scope) error {
		return fmt.Errorf("permission denied")
	}

	deps := &uninstallDeps{
		runner:   newFakeRunner(),
		fetcher:  newFakeFetcher(),
		adapters: []platform.Adapter{adapter},
		stdin:    strings.NewReader(""),
		stdout:   &bytes.Buffer{},
		stderr:   &bytes.Buffer{},
	}

	uninstallYes = true
	installScope = "user"
	targetFlag = ""

	_ = runUninstall("my-plugin", deps)

	out := deps.stdout.(*bytes.Buffer).String()
	assertGoldenString(t, out, "uninstall-error.golden")
}

// ======== Golden Tests: Validate ========

func TestGolden_Validate_PassAll(t *testing.T) {
	dir := t.TempDir()
	manifestData := `dependencies:
  - other-plugin
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	deps := &validateDeps{
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	validateJSON = false
	err := runValidate(deps)
	require.NoError(t, err)

	out := deps.stdout.(*bytes.Buffer).String()
	assertGoldenString(t, out, "validate-pass-all.golden")
}

func TestGolden_Validate_Errors(t *testing.T) {
	dir := t.TempDir()
	manifestData := `system_requirements:
  - nonexistent-binary-xyz-golden-test
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summon.yaml"), []byte(manifestData), 0644))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origDir)

	deps := &validateDeps{
		runner: &execRunner{},
		stdout: &bytes.Buffer{},
	}

	validateJSON = false
	_ = runValidate(deps)

	out := deps.stdout.(*bytes.Buffer).String()
	assertGoldenString(t, out, "validate-errors.golden")
}

// ======== Golden Tests: Self Uninstall ========

func TestGolden_SelfUninstall_Prompt(t *testing.T) {
	selfUninstallConfirm = false

	fs := newFakeSelfUninstallFS()
	fs.statPaths[filepath.Join("/home/user", ".summon")] = true
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/usr/local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader("n\n"),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)

	assertGoldenString(t, stdout.String(), "self-uninstall-prompt.golden")
}

func TestGolden_SelfUninstall_Confirmed(t *testing.T) {
	selfUninstallConfirm = true

	fs := newFakeSelfUninstallFS()
	fs.statPaths[filepath.Join("/home/user", ".summon")] = true
	var stdout bytes.Buffer

	deps := &selfUninstallDeps{
		pathResolver: &fakePathResolver{
			executablePath: "/usr/local/bin/summon",
			homeDir:        "/home/user",
		},
		fileSystem: fs,
		stdin:      strings.NewReader(""),
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
	}

	err := runSelfUninstall(deps)
	require.NoError(t, err)

	assertGoldenString(t, stdout.String(), "self-uninstall-confirmed.golden")
}
