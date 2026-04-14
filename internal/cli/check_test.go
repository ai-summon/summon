package cli

import (
	"bytes"
	"testing"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/stretchr/testify/assert"
)

func TestRunCheck_NoPackages(t *testing.T) {
	_ = setupProjectDir(t)

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

func TestRunCheck_HealthyPackage(t *testing.T) {
	dir := setupProjectDir(t)
	writeRegistryYAML(t, dir, `
summon_version: "0.1.0"
packages:
  my-pkg:
    version: "1.0.0"
    source:
      type: github
      url: https://github.com/test/my-pkg
    platforms: []
`)
	createStorePackage(t, dir, "my-pkg")

	var buf bytes.Buffer
	oldStdout := installer.Stdout
	installer.Stdout = &buf
	t.Cleanup(func() { installer.Stdout = oldStdout })

	checkJSON = false
	checkScope = ""
	checkGlobal = false
	checkProject = false

	err := runCheck(nil, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "my-pkg")
	assert.Contains(t, buf.String(), "all")
}
