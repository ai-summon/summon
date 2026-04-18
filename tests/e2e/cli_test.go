package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ai-summon/summon/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeCLIRunner simulates native CLI commands for E2E tests.
type FakeCLIRunner struct {
	installed map[string]map[string]string // platform → name → source
	commands  [][]string
}

func NewFakeCLIRunner() *FakeCLIRunner {
	return &FakeCLIRunner{
		installed: map[string]map[string]string{
			"copilot": {},
		},
	}
}

func (f *FakeCLIRunner) Run(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, append([]string{name}, args...))

	// Simulate native CLI responses
	if len(args) >= 2 && args[0] == "plugin" {
		switch args[1] {
		case "list":
			platform := name
			plugins, ok := f.installed[platform]
			if !ok {
				return []byte("[]"), nil
			}
			var result []struct {
				Name   string `json:"name"`
				Source string `json:"source"`
			}
			for n, s := range plugins {
				result = append(result, struct {
					Name   string `json:"name"`
					Source string `json:"source"`
				}{Name: n, Source: s})
			}
			data, _ := json.Marshal(result)
			return data, nil

		case "install":
			if len(args) >= 3 {
				platform := name
				if _, ok := f.installed[platform]; !ok {
					f.installed[platform] = make(map[string]string)
				}
				f.installed[platform][args[2]] = args[2]
			}
			return nil, nil

		case "uninstall":
			if len(args) >= 3 {
				platform := name
				delete(f.installed[platform], args[2])
			}
			return nil, nil

		case "update":
			return nil, nil
		}
	}

	return nil, nil
}

func (f *FakeCLIRunner) LookPath(name string) (string, error) {
	if name == "copilot" {
		return "/usr/local/bin/copilot", nil
	}
	return "", fmt.Errorf("%s not found", name)
}

// Test the root command has expected subcommands.
func TestRootCommand_HasSubcommands(t *testing.T) {
	root := cli.GetRootCmd()
	require.NotNil(t, root)

	subCmds := make(map[string]bool)
	for _, cmd := range root.Commands() {
		subCmds[cmd.Name()] = true
	}

	assert.True(t, subCmds["install"], "should have install command")
	assert.True(t, subCmds["uninstall"], "should have uninstall command")
	assert.True(t, subCmds["update"], "should have update command")
	assert.True(t, subCmds["list"], "should have list command")
	assert.True(t, subCmds["check"], "should have check command")
	assert.True(t, subCmds["validate"], "should have validate command")
	assert.True(t, subCmds["marketplace"], "should have marketplace command")
}

// Test help output works.
func TestRootCommand_Help(t *testing.T) {
	root := cli.GetRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "summon")
	assert.Contains(t, buf.String(), "dependency manager for AI plugins")
}

// Test install command help.
func TestInstallCommand_Help(t *testing.T) {
	root := cli.GetRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"install", "--help"})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "dependency resolution")
}
