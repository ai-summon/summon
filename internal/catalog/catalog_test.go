package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- LoadDefault (embedded catalog) ---

func TestLoadDefault(t *testing.T) {
	c, err := LoadDefault()
	require.NoError(t, err)
	assert.NotEmpty(t, c.Entries, "embedded catalog should contain at least one package")
}

func TestLoadDefault_AllEntriesIndexed(t *testing.T) {
	c, err := LoadDefault()
	require.NoError(t, err)
	for _, e := range c.Entries {
		found, ok := c.Lookup(e.Name)
		assert.True(t, ok, "entry not indexed: %s", e.Name)
		assert.Equal(t, e.Repository, found.Repository)
	}
}

func TestLoadDefault_EntriesHaveRequiredFields(t *testing.T) {
	c, err := LoadDefault()
	require.NoError(t, err)
	for _, e := range c.Entries {
		assert.NotEmpty(t, e.Name, "every entry must have a name")
		assert.NotEmpty(t, e.Repository, "entry %q must have a repository", e.Name)
	}
}

// --- Load (custom YAML) ---

func TestLoad_ValidYAML(t *testing.T) {
	yaml := []byte(`packages:
  - name: foo
    repository: https://github.com/org/foo
    description: "Foo package"
  - name: bar
    repository: https://github.com/org/bar
`)
	c, err := Load(yaml)
	require.NoError(t, err)
	assert.Len(t, c.Entries, 2)

	foo, ok := c.Lookup("foo")
	require.True(t, ok)
	assert.Equal(t, "https://github.com/org/foo", foo.Repository)
	assert.Equal(t, "Foo package", foo.Description)

	bar, ok := c.Lookup("bar")
	require.True(t, ok)
	assert.Equal(t, "https://github.com/org/bar", bar.Repository)
	assert.Empty(t, bar.Description, "omitted description should be empty")
}

func TestLoad_EmptyPackagesList(t *testing.T) {
	yaml := []byte("packages: []\n")
	c, err := Load(yaml)
	require.NoError(t, err)
	assert.Empty(t, c.Entries)
}

func TestLoad_EmptyData(t *testing.T) {
	c, err := Load([]byte(""))
	require.NoError(t, err, "empty input is valid YAML (null document)")
	assert.Empty(t, c.Entries)
}

func TestLoad_InvalidYAML(t *testing.T) {
	_, err := Load([]byte(":::invalid"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing catalog")
}

func TestLoad_DuplicateNames_LastEntryWins(t *testing.T) {
	yaml := []byte(`packages:
  - name: dup
    repository: https://github.com/org/first
  - name: dup
    repository: https://github.com/org/second
`)
	c, err := Load(yaml)
	require.NoError(t, err)
	assert.Len(t, c.Entries, 2, "raw list preserves both entries")

	entry, ok := c.Lookup("dup")
	require.True(t, ok)
	assert.Equal(t, "https://github.com/org/second", entry.Repository,
		"index should keep the last entry for a duplicate name")
}

// --- Lookup ---

func TestLookup_Found(t *testing.T) {
	c, err := LoadDefault()
	require.NoError(t, err)

	// Pick a known entry from the embedded catalog.
	entry, ok := c.Lookup("superpowers")
	assert.True(t, ok)
	assert.Equal(t, "superpowers", entry.Name)
	assert.Contains(t, entry.Repository, "github.com")
}

func TestLookup_NotFound(t *testing.T) {
	c, err := LoadDefault()
	require.NoError(t, err)

	_, ok := c.Lookup("nonexistent-package")
	assert.False(t, ok)
}

func TestLookup_CaseSensitive(t *testing.T) {
	yaml := []byte(`packages:
  - name: MyTool
    repository: https://github.com/org/mytool
`)
	c, err := Load(yaml)
	require.NoError(t, err)

	_, ok := c.Lookup("MyTool")
	assert.True(t, ok, "exact case should match")

	_, ok = c.Lookup("mytool")
	assert.False(t, ok, "different case should not match")

	_, ok = c.Lookup("MYTOOL")
	assert.False(t, ok, "different case should not match")
}

func TestLookup_EmptyCatalog(t *testing.T) {
	c, err := Load([]byte("packages: []\n"))
	require.NoError(t, err)

	_, ok := c.Lookup("anything")
	assert.False(t, ok, "lookup on empty catalog should return false")
}
