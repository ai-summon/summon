package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-summon/summon/internal/marketplace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMarketplaceBrowseWith_LocalCache(t *testing.T) {
	var buf bytes.Buffer
	deps := &browseDeps{
		stdout:  &buf,
		noColor: true,
		localReader: func(name string) (marketplace.Index, error) {
			return marketplace.Index{
				"alpha-tool":  {Source: "gh:owner/alpha-tool", Description: "Alpha description"},
				"beta-plugin": {Source: "gh:owner/beta-plugin", Description: "Beta description"},
			}, nil
		},
	}

	err := runMarketplaceBrowseWith("summon-marketplace", deps)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Packages in summon-marketplace:")
	assert.Contains(t, output, "alpha-tool")
	assert.Contains(t, output, "Alpha description")
	assert.Contains(t, output, "beta-plugin")
	assert.Contains(t, output, "Beta description")
	assert.Contains(t, output, "2 package(s) available")
}

func TestRunMarketplaceBrowseWith_SortedAlphabetically(t *testing.T) {
	var buf bytes.Buffer
	deps := &browseDeps{
		stdout:  &buf,
		noColor: true,
		localReader: func(name string) (marketplace.Index, error) {
			return marketplace.Index{
				"zulu":  {Description: "Last"},
				"alpha": {Description: "First"},
				"mike":  {Description: "Middle"},
			}, nil
		},
	}

	err := runMarketplaceBrowseWith("test-marketplace", deps)
	require.NoError(t, err)

	output := buf.String()
	alphaIdx := bytes.Index([]byte(output), []byte("alpha"))
	mikeIdx := bytes.Index([]byte(output), []byte("mike"))
	zuluIdx := bytes.Index([]byte(output), []byte("zulu"))
	assert.Less(t, alphaIdx, mikeIdx)
	assert.Less(t, mikeIdx, zuluIdx)
}

func TestRunMarketplaceBrowseWith_FallbackToHTTP(t *testing.T) {
	var buf bytes.Buffer
	localCalled := false
	fetcherCalled := false

	deps := &browseDeps{
		stdout:     &buf,
		noColor:    true,
		configPath: "/nonexistent/config.yaml",
		localReader: func(name string) (marketplace.Index, error) {
			localCalled = true
			return nil, assert.AnError
		},
		fetcher: &fakeIndexFetcherForBrowse{
			index: marketplace.Index{
				"remote-plugin": {Source: "gh:owner/remote", Description: "From remote"},
			},
			onFetch: func() { fetcherCalled = true },
		},
	}

	err := runMarketplaceBrowseWith("summon-marketplace", deps)
	require.NoError(t, err)

	assert.True(t, localCalled)
	assert.True(t, fetcherCalled)
	assert.Contains(t, buf.String(), "remote-plugin")
	assert.Contains(t, buf.String(), "From remote")
}

func TestRunMarketplaceBrowseWith_MarketplaceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	// Empty config
	require.NoError(t, os.WriteFile(configPath, []byte("marketplaces: []\n"), 0644))

	var buf bytes.Buffer
	deps := &browseDeps{
		stdout:     &buf,
		noColor:    true,
		configPath: configPath,
		localReader: func(name string) (marketplace.Index, error) {
			return nil, assert.AnError
		},
	}

	err := runMarketplaceBrowseWith("unknown-marketplace", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunMarketplaceBrowseWith_EmptyIndex(t *testing.T) {
	var buf bytes.Buffer
	deps := &browseDeps{
		stdout:  &buf,
		noColor: true,
		localReader: func(name string) (marketplace.Index, error) {
			return marketplace.Index{}, nil
		},
	}

	err := runMarketplaceBrowseWith("empty-marketplace", deps)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No packages found")
}

func TestRunMarketplaceBrowseWith_UserRegisteredMarketplace(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configData := "marketplaces:\n  - name: my-marketplace\n    source: https://github.com/org/my-marketplace\n"
	require.NoError(t, os.WriteFile(configPath, []byte(configData), 0644))

	var buf bytes.Buffer
	deps := &browseDeps{
		stdout:     &buf,
		noColor:    true,
		configPath: configPath,
		localReader: func(name string) (marketplace.Index, error) {
			return nil, assert.AnError
		},
		fetcher: &fakeIndexFetcherForBrowse{
			index: marketplace.Index{
				"custom-tool": {Source: "gh:org/custom-tool", Description: "Custom tool"},
			},
		},
	}

	err := runMarketplaceBrowseWith("my-marketplace", deps)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "custom-tool")
	assert.Contains(t, buf.String(), "Custom tool")
}

func TestReadLocalIndex_WithLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create Claude cache structure
	mktDir := filepath.Join(tmpDir, ".claude", "plugins", "marketplaces", "test-mkt", ".claude-plugin")
	require.NoError(t, os.MkdirAll(mktDir, 0755))

	indexJSON := `{
		"name": "test-mkt",
		"owner": {"name": "Test"},
		"metadata": {"description": "Test marketplace", "version": "0.1.0"},
		"plugins": [
			{
				"name": "local-plugin",
				"source": {"source": "github", "repo": "owner/local-plugin"},
				"description": "Locally cached plugin"
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(mktDir, "marketplace.json"), []byte(indexJSON), 0644))

	idx, err := marketplace.ReadLocalIndexWithHome("test-mkt", tmpDir)
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("local-plugin")
	assert.True(t, found)
	assert.Equal(t, "gh:owner/local-plugin", entry.Source)
}

func TestReadLocalIndex_NoLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := marketplace.ReadLocalIndexWithHome("nonexistent-mkt", tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no local cache found")
}

// --- Fake IndexFetcher for browse tests ---

type fakeIndexFetcherForBrowse struct {
	index   marketplace.Index
	err     error
	onFetch func()
}

func (f *fakeIndexFetcherForBrowse) FetchMarketplaceIndex(source string) (marketplace.Index, error) {
	if f.onFetch != nil {
		f.onFetch()
	}
	return f.index, f.err
}

func (f *fakeIndexFetcherForBrowse) LookupPackage(name string, source string) (*marketplace.PackageEntry, error) {
	if f.index != nil {
		entry, ok := f.index.Lookup(name)
		if ok {
			return entry, nil
		}
	}
	return nil, assert.AnError
}
