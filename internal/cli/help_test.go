package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ======== Help Output Golden Tests ========

func TestGolden_RootHelp(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	assertGoldenString(t, buf.String(), "help-root.golden")
}

func TestGolden_InstallHelp(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"install", "--help"})
	err := root.Execute()
	require.NoError(t, err)

	assertGoldenString(t, buf.String(), "help-install.golden")
}

func TestGolden_MarketplaceHelp(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"marketplace", "--help"})
	err := root.Execute()
	require.NoError(t, err)

	assertGoldenString(t, buf.String(), "help-marketplace.golden")
}

func TestGolden_SelfHelp(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"self", "--help"})
	err := root.Execute()
	require.NoError(t, err)

	assertGoldenString(t, buf.String(), "help-self.golden")
}

// ======== Structural Tests ========

func TestHelp_RootHasGroupedCommands(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())

	out := buf.String()

	// Verify groups appear in order
	groups := []string{"Plugin Management:", "Inspection:", "Configuration:", "Maintenance:", "Plugin Development:"}
	lastIdx := -1
	for _, g := range groups {
		idx := strings.Index(out, g)
		assert.Greater(t, idx, lastIdx, "group %q should appear after previous group", g)
		lastIdx = idx
	}

	// Verify no "Available Commands:" or "Additional Commands:" sections
	assert.NotContains(t, out, "Available Commands:")
	assert.NotContains(t, out, "Additional Commands:")
}

func TestHelp_NoColorProducesCleanOutput(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())

	out := buf.String()

	// No ANSI escape codes
	assert.NotContains(t, out, "\x1b[", "no-color output should not contain ANSI escape codes")
	assert.NotContains(t, out, "\033[", "no-color output should not contain ANSI escape codes")
}

func TestHelp_SubcommandInheritsStyle(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"marketplace", "--help"})
	require.NoError(t, root.Execute())

	out := buf.String()
	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "Available Commands:")
	assert.Contains(t, out, "Flags:")
}

func TestHelp_InstallShowsExamples(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	var buf bytes.Buffer
	root := GetRootCmd()
	root.SetOut(&buf)
	root.SetArgs([]string{"install", "--help"})
	require.NoError(t, root.Execute())

	out := buf.String()
	assert.Contains(t, out, "Examples:")
	assert.Contains(t, out, "summon install my-plugin")
	assert.Contains(t, out, "gh:owner/my-plugin")
}

// ======== Flag Styling Tests ========

func TestStyleFlagUsages_BasicFormatting(t *testing.T) {
	input := "  -h, --help           help for summon\n      --no-color        Disable colored output"
	identity := func(s string) string { return s }

	result := styleFlagUsages(input, identity)
	// Identity function should not change the output
	assert.Equal(t, input, result)
}

func TestStyleFlagLine_NoDescription(t *testing.T) {
	identity := func(s string) string { return s }

	line := "      --verbose"
	result := styleFlagLine(line, identity)
	assert.Equal(t, line, result)
}

func TestStyleFlagLine_WithDescription(t *testing.T) {
	called := false
	marker := func(s string) string {
		called = true
		return "[" + s + "]"
	}

	line := "  -h, --help           help for summon"
	result := styleFlagLine(line, marker)
	assert.True(t, called, "style function should be called")
	assert.Contains(t, result, "[-h, --help")
	assert.Contains(t, result, "help for summon")
}

func TestFindDescriptionStart_MultipleSpaces(t *testing.T) {
	line := "  -h, --help           help for summon"
	idx := findDescriptionStart(line)
	assert.Greater(t, idx, 0)
	assert.Equal(t, "help for summon", line[idx:])
}

func TestFindDescriptionStart_NoDescription(t *testing.T) {
	line := "      --verbose"
	idx := findDescriptionStart(line)
	assert.Equal(t, -1, idx)
}

func TestFindDescriptionStart_EmptyLine(t *testing.T) {
	idx := findDescriptionStart("")
	assert.Equal(t, -1, idx)
}
