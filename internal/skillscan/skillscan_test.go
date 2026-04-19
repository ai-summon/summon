package skillscan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ParseSkillName tests ---

func TestParseSkillName_Valid(t *testing.T) {
	data := []byte("---\nname: my-skill\ndescription: A test skill\n---\nBody content here.\n")
	name, err := ParseSkillName(data)
	require.NoError(t, err)
	assert.Equal(t, "my-skill", name)
}

func TestParseSkillName_QuotedName(t *testing.T) {
	data := []byte("---\nname: \"init\"\ndescription: \"Initialize\"\n---\n")
	name, err := ParseSkillName(data)
	require.NoError(t, err)
	assert.Equal(t, "init", name)
}

func TestParseSkillName_WithMetadata(t *testing.T) {
	data := []byte("---\nname: brainstorm\ndescription: \"A brainstorming skill\"\nargument-hint: \"optional args\"\nuser-invocable: true\nmetadata:\n  foo: bar\n---\nContent.\n")
	name, err := ParseSkillName(data)
	require.NoError(t, err)
	assert.Equal(t, "brainstorm", name)
}

func TestParseSkillName_EmptyFile(t *testing.T) {
	_, err := ParseSkillName([]byte(""))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty file")
}

func TestParseSkillName_NoFrontmatter(t *testing.T) {
	_, err := ParseSkillName([]byte("Just some text without frontmatter"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing frontmatter delimiter")
}

func TestParseSkillName_EmptyFrontmatter(t *testing.T) {
	_, err := ParseSkillName([]byte("---\n---\n"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty frontmatter")
}

func TestParseSkillName_MissingName(t *testing.T) {
	_, err := ParseSkillName([]byte("---\ndescription: No name here\n---\n"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'name' field")
}

func TestParseSkillName_MalformedYAML(t *testing.T) {
	data := []byte("---\n[invalid yaml\n---\n")
	_, err := ParseSkillName(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid frontmatter YAML")
}

// --- FindPluginManifest tests ---

func TestFindPluginManifest_ClaudePlugin(t *testing.T) {
	dir := t.TempDir()
	cpDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(`{"name":"test"}`), 0o644))

	path, err := FindPluginManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cpDir, "plugin.json"), path)
}

func TestFindPluginManifest_DotPlugin(t *testing.T) {
	dir := t.TempDir()
	dpDir := filepath.Join(dir, ".plugin")
	require.NoError(t, os.MkdirAll(dpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dpDir, "plugin.json"), []byte(`{"name":"test"}`), 0o644))

	path, err := FindPluginManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dpDir, "plugin.json"), path)
}

func TestFindPluginManifest_RootPluginJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"test"}`), 0o644))

	path, err := FindPluginManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "plugin.json"), path)
}

func TestFindPluginManifest_Precedence(t *testing.T) {
	dir := t.TempDir()
	// Create both .plugin/plugin.json and .claude-plugin/plugin.json
	for _, sub := range []string{".plugin", ".claude-plugin"} {
		d := filepath.Join(dir, sub)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "plugin.json"), []byte(`{"name":"test"}`), 0o644))
	}

	path, err := FindPluginManifest(dir)
	require.NoError(t, err)
	// .plugin should win over .claude-plugin
	assert.Equal(t, filepath.Join(dir, ".plugin", "plugin.json"), path)
}

func TestFindPluginManifest_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindPluginManifest(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no plugin manifest found")
}

// --- ReadSkillDirs tests ---

func TestReadSkillDirs_Default(t *testing.T) {
	dir := t.TempDir()
	// No manifest — should return default
	dirs, err := ReadSkillDirs(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "skills")}, dirs)
}

func TestReadSkillDirs_StringField(t *testing.T) {
	dir := t.TempDir()
	cpDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(`{"name":"test","skills":"custom-skills/"}`), 0o644))

	dirs, err := ReadSkillDirs(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "custom-skills/")}, dirs)
}

func TestReadSkillDirs_ArrayField(t *testing.T) {
	dir := t.TempDir()
	cpDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(`{"name":"test","skills":["skills/","extra/"]}`), 0o644))

	dirs, err := ReadSkillDirs(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "skills/"), filepath.Join(dir, "extra/")}, dirs)
}

func TestReadSkillDirs_NullSkillsField(t *testing.T) {
	dir := t.TempDir()
	cpDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(`{"name":"test","skills":null}`), 0o644))

	dirs, err := ReadSkillDirs(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "skills")}, dirs)
}

func TestReadSkillDirs_NoSkillsField(t *testing.T) {
	dir := t.TempDir()
	cpDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(`{"name":"test"}`), 0o644))

	dirs, err := ReadSkillDirs(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "skills")}, dirs)
}

// --- ScanPlugin tests ---

func createTestPlugin(t *testing.T, dir string, skills map[string]string) {
	t.Helper()
	skillsDir := filepath.Join(dir, "skills")
	for skillDir, frontmatter := range skills {
		d := filepath.Join(skillsDir, skillDir)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(frontmatter), 0o644))
	}
}

func TestScanPlugin_Normal(t *testing.T) {
	dir := t.TempDir()
	createTestPlugin(t, dir, map[string]string{
		"init":       "---\nname: init\ndescription: Initialize\n---\n",
		"brainstorm": "---\nname: brainstorm\ndescription: Think\n---\n",
	})

	entries, scanErrors := ScanPlugin(dir, "wingman", "summon-marketplace", 0)
	assert.Empty(t, scanErrors)
	assert.Len(t, entries, 2)

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
		assert.Equal(t, "wingman", e.PluginName)
		assert.Equal(t, "summon-marketplace", e.Marketplace)
		assert.Equal(t, 0, e.Order)
	}
	assert.True(t, names["init"])
	assert.True(t, names["brainstorm"])
}

func TestScanPlugin_EmptySkillsDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills"), 0o755))

	entries, scanErrors := ScanPlugin(dir, "empty", "mkt", 0)
	assert.Empty(t, scanErrors)
	assert.Empty(t, entries)
}

func TestScanPlugin_NoSkillsDir(t *testing.T) {
	dir := t.TempDir()

	entries, scanErrors := ScanPlugin(dir, "noskills", "mkt", 0)
	assert.Empty(t, scanErrors) // missing dir is not an error
	assert.Empty(t, entries)
}

func TestScanPlugin_MalformedSkillMD(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "broken")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("no frontmatter"), 0o644))

	entries, scanErrors := ScanPlugin(dir, "broken-plugin", "mkt", 0)
	assert.Empty(t, entries)
	assert.Len(t, scanErrors, 1)
	assert.Contains(t, scanErrors[0].Error(), "parsing")
}

func TestScanPlugin_SkipNonDirectories(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	// Create a file (not directory) in skills/
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "README.md"), []byte("readme"), 0o644))
	// Create a valid skill directory
	createTestPlugin(t, dir, map[string]string{
		"valid": "---\nname: valid\n---\n",
	})

	entries, scanErrors := ScanPlugin(dir, "p", "m", 0)
	assert.Empty(t, scanErrors)
	assert.Len(t, entries, 1)
	assert.Equal(t, "valid", entries[0].Name)
}

func TestScanPlugin_CustomSkillPaths(t *testing.T) {
	dir := t.TempDir()
	// Plugin.json with custom skills path
	cpDir := filepath.Join(dir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(cpDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cpDir, "plugin.json"), []byte(`{"name":"test","skills":["skills/","extra/"]}`), 0o644))
	// Create skills in both directories
	createTestPlugin(t, dir, map[string]string{
		"main-skill": "---\nname: main-skill\n---\n",
	})
	extraDir := filepath.Join(dir, "extra", "bonus")
	require.NoError(t, os.MkdirAll(extraDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extraDir, "SKILL.md"), []byte("---\nname: bonus-skill\n---\n"), 0o644))

	entries, scanErrors := ScanPlugin(dir, "multi", "mkt", 0)
	assert.Empty(t, scanErrors)
	assert.Len(t, entries, 2)

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	assert.True(t, names["main-skill"])
	assert.True(t, names["bonus-skill"])
}

func TestScanPlugin_RelativePath(t *testing.T) {
	dir := t.TempDir()
	createTestPlugin(t, dir, map[string]string{
		"hello": "---\nname: hello\n---\n",
	})

	entries, _ := ScanPlugin(dir, "p", "m", 0)
	require.Len(t, entries, 1)
	assert.Equal(t, filepath.Join("skills", "hello", "SKILL.md"), entries[0].FilePath)
}

// --- DetectCollisions tests ---

func TestDetectCollisions_NoCollisions(t *testing.T) {
	entries := []SkillEntry{
		{Name: "init", PluginName: "wingman", Order: 0},
		{Name: "brainstorm", PluginName: "brainstorm-plugin", Order: 1},
		{Name: "reader", PluginName: "mcap", Order: 2},
	}

	collisions := DetectCollisions(entries)
	assert.Empty(t, collisions)
}

func TestDetectCollisions_SingleCollision(t *testing.T) {
	entries := []SkillEntry{
		{Name: "init", PluginName: "wingman", Marketplace: "summon-marketplace", Order: 0},
		{Name: "init", PluginName: "speckit", Marketplace: "summon-marketplace", Order: 2},
		{Name: "reader", PluginName: "mcap", Order: 1},
	}

	collisions := DetectCollisions(entries)
	require.Len(t, collisions, 1)
	assert.Equal(t, "init", collisions[0].SkillName)
	assert.Len(t, collisions[0].Entries, 2)
	// Winner (lower order) should be first
	assert.Equal(t, "wingman", collisions[0].Entries[0].PluginName)
	assert.Equal(t, "speckit", collisions[0].Entries[1].PluginName)
}

func TestDetectCollisions_MultipleCollisions(t *testing.T) {
	entries := []SkillEntry{
		{Name: "init", PluginName: "wingman", Order: 0},
		{Name: "brainstorm", PluginName: "wingman", Order: 0},
		{Name: "init", PluginName: "speckit", Order: 2},
		{Name: "brainstorm", PluginName: "brainstorm-plugin", Order: 1},
	}

	collisions := DetectCollisions(entries)
	require.Len(t, collisions, 2)
	// Sorted by skill name
	assert.Equal(t, "brainstorm", collisions[0].SkillName)
	assert.Equal(t, "init", collisions[1].SkillName)
}

func TestDetectCollisions_SamePluginDuplicate(t *testing.T) {
	entries := []SkillEntry{
		{Name: "hello", PluginName: "my-plugin", Marketplace: "mkt", FilePath: "skills/hello/SKILL.md", Order: 0},
		{Name: "hello", PluginName: "my-plugin", Marketplace: "mkt", FilePath: "extra/hello/SKILL.md", Order: 0},
	}

	collisions := DetectCollisions(entries)
	require.Len(t, collisions, 1)
	assert.Equal(t, "hello", collisions[0].SkillName)
	assert.Len(t, collisions[0].Entries, 2)
}

func TestDetectCollisions_ThreeWayCollision(t *testing.T) {
	entries := []SkillEntry{
		{Name: "deploy", PluginName: "plugin-c", Order: 2},
		{Name: "deploy", PluginName: "plugin-a", Order: 0},
		{Name: "deploy", PluginName: "plugin-b", Order: 1},
	}

	collisions := DetectCollisions(entries)
	require.Len(t, collisions, 1)
	assert.Equal(t, "deploy", collisions[0].SkillName)
	require.Len(t, collisions[0].Entries, 3)
	assert.Equal(t, "plugin-a", collisions[0].Entries[0].PluginName)
	assert.Equal(t, "plugin-b", collisions[0].Entries[1].PluginName)
	assert.Equal(t, "plugin-c", collisions[0].Entries[2].PluginName)
}

func TestDetectCollisions_Empty(t *testing.T) {
	collisions := DetectCollisions(nil)
	assert.Empty(t, collisions)
}

func TestDetectCollisions_OrderTiebreaksByName(t *testing.T) {
	entries := []SkillEntry{
		{Name: "test", PluginName: "zebra", Order: 0},
		{Name: "test", PluginName: "alpha", Order: 0},
	}

	collisions := DetectCollisions(entries)
	require.Len(t, collisions, 1)
	// Same order, alphabetical by plugin name
	assert.Equal(t, "alpha", collisions[0].Entries[0].PluginName)
	assert.Equal(t, "zebra", collisions[0].Entries[1].PluginName)
}
