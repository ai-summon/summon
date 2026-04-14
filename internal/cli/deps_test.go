package cli

import (
	"bytes"
	"testing"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/stretchr/testify/assert"
)

func TestRunDeps_PackageInfo(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  code-reviewer:
    version: "2.1.0"
    source: {type: github, url: "https://github.com/test/code-reviewer"}
    platforms: [claude]
`)
	createStorePackage(t, dir, "code-reviewer")

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	installer.Stdout = &buf
	t.Cleanup(func() { installer.Stdout = oldStdout })

	depsJSON = false
	depsScope = "local"
	depsGlobal = false
	depsProject = false

	err := runDeps(nil, []string{"code-reviewer"})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "code-reviewer")
	assert.Contains(t, buf.String(), "2.1.0")
}

func TestRunDeps_PackageNotFound(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages: {}
`)

	depsJSON = false
	depsScope = "local"
	depsGlobal = false
	depsProject = false

	err := runDeps(nil, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestRunDeps_JSON(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  code-reviewer:
    version: "2.1.0"
    source: {type: github, url: "https://github.com/test/code-reviewer"}
    platforms: [claude]
`)
	createStorePackage(t, dir, "code-reviewer")

	out := captureStdout(t, func() {
		depsJSON = true
		depsScope = "local"
		depsGlobal = false
		depsProject = false
		_ = runDeps(nil, []string{"code-reviewer"})
	})

	assert.Contains(t, out, `"name"`)
	assert.Contains(t, out, `"code-reviewer"`)
}
