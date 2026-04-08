package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeManifest(t *testing.T, dir, scope, name, content string) {
	t.Helper()
	pkgDir := filepath.Join(dir, ".summon", scope, "store", name)
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "summon.yaml"),
		[]byte(content), 0o644,
	))
}

func TestRunCheck_AllSatisfied(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  pkg-a:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/pkg-a"}
  dep-x:
    version: "2.0.0"
    source: {type: github, url: "https://github.com/test/dep-x"}
`)
	writeManifest(t, dir, "local", "pkg-a", `
name: pkg-a
version: "1.0.0"
description: "test"
dependencies:
  dep-x: ">=1.0.0"
`)
	writeManifest(t, dir, "local", "dep-x", `
name: dep-x
version: "2.0.0"
description: "test"
`)

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	installer.Stdout = &buf
	t.Cleanup(func() { installer.Stdout = oldStdout })

	// Mock exit to prevent os.Exit
	checkJSON = false
	checkScope = "local"
	checkGlobal = false
	checkProject = false

	err := runCheck(nil, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "✓ pkg-a")
}

func TestRunCheck_MissingDep(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  pkg-a:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/pkg-a"}
`)
	writeManifest(t, dir, "local", "pkg-a", `
name: pkg-a
version: "1.0.0"
description: "test"
dependencies:
  missing-dep: ">=1.0.0"
`)

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	oldStderr := installer.Stderr
	installer.Stdout = &buf
	installer.Stderr = &buf
	t.Cleanup(func() {
		installer.Stdout = oldStdout
		installer.Stderr = oldStderr
	})

	checkJSON = false
	checkScope = "local"
	checkGlobal = false
	checkProject = false

	err := runCheck(nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsatisfied dependencies")
}

func TestRunCheck_JSON(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  pkg-a:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/pkg-a"}
  dep-x:
    version: "2.0.0"
    source: {type: github, url: "https://github.com/test/dep-x"}
`)
	writeManifest(t, dir, "local", "pkg-a", `
name: pkg-a
version: "1.0.0"
description: "test"
dependencies:
  dep-x: ">=1.0.0"
`)
	writeManifest(t, dir, "local", "dep-x", `
name: dep-x
version: "2.0.0"
description: "test"
`)

	// Capture stdout for JSON output
	out := captureStdout(t, func() {
		checkJSON = true
		checkScope = "local"
		checkGlobal = false
		checkProject = false
		_ = runCheck(nil, nil)
	})

	assert.Contains(t, out, `"all_satisfied": true`)
	assert.Contains(t, out, `"package_name": "pkg-a"`)
}

func TestRunCheck_NoPackages(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages: {}
`)

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	installer.Stdout = &buf
	t.Cleanup(func() { installer.Stdout = oldStdout })

	checkJSON = false
	checkScope = "local"
	checkGlobal = false
	checkProject = false

	err := runCheck(nil, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "no packages installed")
}

func TestRunCheck_VersionMismatch(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  pkg-a:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/pkg-a"}
  dep-x:
    version: "1.5.0"
    source: {type: github, url: "https://github.com/test/dep-x"}
`)
	writeManifest(t, dir, "local", "pkg-a", `
name: pkg-a
version: "1.0.0"
description: "test"
dependencies:
  dep-x: ">=2.0.0"
`)
	writeManifest(t, dir, "local", "dep-x", `
name: dep-x
version: "1.5.0"
description: "test"
`)

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	oldStderr := installer.Stderr
	installer.Stdout = &buf
	installer.Stderr = &buf
	t.Cleanup(func() {
		installer.Stdout = oldStdout
		installer.Stderr = oldStderr
	})

	checkJSON = false
	checkScope = "local"
	checkGlobal = false
	checkProject = false

	err := runCheck(nil, nil)
	assert.Error(t, err)
	assert.Contains(t, buf.String(), "✗ pkg-a")
}
