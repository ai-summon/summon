package marketplace

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchIndex_ValidJSON(t *testing.T) {
	data := `{
		"my-plugin": {
			"source": "https://github.com/owner/my-plugin",
			"description": "A useful plugin"
		},
		"other-tool": {
			"source": "gh:owner/other-tool",
			"description": "Another tool"
		}
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 2)
}

func TestFetchIndex_MalformedJSON(t *testing.T) {
	_, err := FetchIndex([]byte("{invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestLookup_ExistingPackage(t *testing.T) {
	idx := Index{
		"my-plugin": {Source: "https://github.com/owner/my-plugin", Description: "A plugin"},
	}
	entry, found := idx.Lookup("my-plugin")
	assert.True(t, found)
	assert.Equal(t, "https://github.com/owner/my-plugin", entry.Source)
	assert.Equal(t, "A plugin", entry.Description)
}

func TestLookup_MissingPackage(t *testing.T) {
	idx := Index{}
	_, found := idx.Lookup("nonexistent")
	assert.False(t, found)
}

func TestFetchIndex_EmptyIndex(t *testing.T) {
	idx, err := FetchIndex([]byte("{}"))
	require.NoError(t, err)
	assert.Empty(t, idx)
}

func TestPackageEntry_JSONRoundTrip(t *testing.T) {
	entry := PackageEntry{
		Source:      "https://github.com/owner/repo",
		Description: "Test description",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded PackageEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, entry, decoded)
}
