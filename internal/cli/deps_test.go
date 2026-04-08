package cli

import (
	"bytes"
	"testing"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/stretchr/testify/assert"
)

func TestRunDeps_WithDeps(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  code-reviewer:
    version: "2.1.0"
    source: {type: github, url: "https://github.com/test/code-reviewer"}
  prompt-library:
    version: "1.5.0"
    source: {type: github, url: "https://github.com/test/prompt-library"}
`)
	writeManifest(t, dir, "local", "code-reviewer", `
name: code-reviewer
version: "2.1.0"
description: "test"
dependencies:
  prompt-library: "^1.0.0"
`)
	writeManifest(t, dir, "local", "prompt-library", `
name: prompt-library
version: "1.5.0"
description: "test"
`)

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
	assert.Contains(t, buf.String(), "✓ prompt-library")
	assert.Contains(t, buf.String(), "1 of 1 dependencies satisfied")
}

func TestRunDeps_NoDeps(t *testing.T) {
	dir := setupProjectDir(t)
	writeScopedRegistryYAML(t, dir, "local", `
summon_version: "0.1.0"
packages:
  standalone:
    version: "1.0.0"
    source: {type: github, url: "https://github.com/test/standalone"}
`)
	writeManifest(t, dir, "local", "standalone", `
name: standalone
version: "1.0.0"
description: "test"
`)

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	installer.Stdout = &buf
	t.Cleanup(func() { installer.Stdout = oldStdout })

	depsJSON = false
	depsScope = "local"
	depsGlobal = false
	depsProject = false

	err := runDeps(nil, []string{"standalone"})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "has no dependencies")
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
  prompt-library:
    version: "1.5.0"
    source: {type: github, url: "https://github.com/test/prompt-library"}
`)
	writeManifest(t, dir, "local", "code-reviewer", `
name: code-reviewer
version: "2.1.0"
description: "test"
dependencies:
  prompt-library: "^1.0.0"
`)
	writeManifest(t, dir, "local", "prompt-library", `
name: prompt-library
version: "1.5.0"
description: "test"
`)

	out := captureStdout(t, func() {
		depsJSON = true
		depsScope = "local"
		depsGlobal = false
		depsProject = false
		_ = runDeps(nil, []string{"code-reviewer"})
	})

	assert.Contains(t, out, `"package_name": "code-reviewer"`)
	assert.Contains(t, out, `"all_satisfied": true`)
}
