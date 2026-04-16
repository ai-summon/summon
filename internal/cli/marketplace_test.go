package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/marketplace"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Browse tests ---

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
		stdout:  &buf,
		noColor: true,
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

	// Official marketplace resolves source without adapters
	err := runMarketplaceBrowseWith("summon-marketplace", deps)
	require.NoError(t, err)

	assert.True(t, localCalled)
	assert.True(t, fetcherCalled)
	assert.Contains(t, buf.String(), "remote-plugin")
	assert.Contains(t, buf.String(), "From remote")
}

func TestRunMarketplaceBrowseWith_FallbackUsesAdapter(t *testing.T) {
	var buf bytes.Buffer
	adapter := newFakeAdapter("copilot")
	adapter.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return []platform.MarketplaceInfo{
			{Name: "my-marketplace", Source: "https://github.com/org/my-marketplace"},
		}, nil
	}

	deps := &browseDeps{
		stdout:  &buf,
		noColor: true,
		adapters: []platform.Adapter{adapter},
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

func TestRunMarketplaceBrowseWith_MarketplaceNotFound(t *testing.T) {
	var buf bytes.Buffer
	adapter := newFakeAdapter("copilot")
	adapter.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return nil, nil
	}

	deps := &browseDeps{
		stdout:   &buf,
		noColor:  true,
		adapters: []platform.Adapter{adapter},
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

func TestReadLocalIndex_WithLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
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

// --- Add tests ---

func TestRunMarketplaceAddWith_DelegatesToAdapters(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	adapter1 := newFakeAdapter("copilot")
	adapter2 := newFakeAdapter("claude")

	var ensured []string
	ensureFunc := func(name, source string) error {
		ensured = append(ensured, name+"@"+source)
		return nil
	}
	adapter1.ensureMarketplaceFunc = ensureFunc
	adapter2.ensureMarketplaceFunc = ensureFunc

	deps := &addDeps{
		stdout:   &outBuf,
		stderr:   &errBuf,
		adapters: []platform.Adapter{adapter1, adapter2},
	}

	err := runMarketplaceAddWith("https://github.com/org/my-marketplace", deps)
	require.NoError(t, err)

	assert.Len(t, ensured, 2)
	assert.Contains(t, ensured[0], "my-marketplace@")
	assert.Contains(t, ensured[1], "my-marketplace@")

	output := outBuf.String()
	assert.Contains(t, output, `Registering marketplace "my-marketplace"`)
	assert.Contains(t, output, "✓ copilot: marketplace registered")
	assert.Contains(t, output, "✓ claude: marketplace registered")
}

func TestRunMarketplaceAddWith_ContinuesOnAdapterFailure(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	failAdapter := newFakeAdapter("copilot")
	failAdapter.ensureMarketplaceFunc = func(name, source string) error {
		return fmt.Errorf("copilot not available")
	}
	okAdapter := newFakeAdapter("claude")
	okAdapter.ensureMarketplaceFunc = func(name, source string) error {
		return nil
	}

	deps := &addDeps{
		stdout:   &outBuf,
		stderr:   &errBuf,
		adapters: []platform.Adapter{failAdapter, okAdapter},
	}

	err := runMarketplaceAddWith("https://github.com/org/test-mkt", deps)
	require.NoError(t, err)

	assert.Contains(t, errBuf.String(), "⚠ copilot: failed to register marketplace")
	assert.Contains(t, outBuf.String(), "✓ claude: marketplace registered")
}

func TestRunMarketplaceAddWith_NoAdapters(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	deps := &addDeps{
		stdout:   &outBuf,
		stderr:   &errBuf,
		adapters: nil,
	}

	err := runMarketplaceAddWith("https://github.com/org/solo-mkt", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs detected")
}

// --- List tests ---

func TestRunMarketplaceListWith_GroupsByPlatform(t *testing.T) {
	var buf bytes.Buffer
	a1 := newFakeAdapter("copilot")
	a1.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return []platform.MarketplaceInfo{
			{Name: "summon-marketplace", Source: "https://github.com/ai-summon/summon-marketplace"},
			{Name: "my-mkt", Source: "https://github.com/org/my-mkt"},
		}, nil
	}
	a2 := newFakeAdapter("claude")
	a2.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return []platform.MarketplaceInfo{
			{Name: "summon-marketplace", Source: "https://github.com/ai-summon/summon-marketplace"},
		}, nil
	}

	deps := &marketplaceListDeps{
		stdout:   &buf,
		noColor:  true,
		adapters: []platform.Adapter{a1, a2},
	}

	err := runMarketplaceListWith(deps)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Copilot (2):")
	assert.Contains(t, output, "Claude (1):")
	// summon-marketplace appears in both sections (count lines containing the icon + name pattern)
	lines := strings.Split(output, "\n")
	mktCount := 0
	myMktCount := 0
	for _, line := range lines {
		if strings.Contains(line, "summon-marketplace") && (strings.Contains(line, "★") || strings.Contains(line, "●")) {
			mktCount++
		}
		if strings.Contains(line, "my-mkt") && strings.Contains(line, "●") {
			myMktCount++
		}
	}
	assert.Equal(t, 2, mktCount, "summon-marketplace should appear in both platform sections")
	assert.Equal(t, 1, myMktCount, "my-mkt should appear only in copilot section")
	// No total count line
	assert.NotContains(t, output, "marketplace(s) registered")
}

func TestRunMarketplaceListWith_SingleAdapter(t *testing.T) {
	var buf bytes.Buffer
	a := newFakeAdapter("copilot")
	a.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return []platform.MarketplaceInfo{
			{Name: "my-mkt", Source: "https://github.com/org/my-mkt"},
		}, nil
	}

	deps := &marketplaceListDeps{
		stdout:   &buf,
		noColor:  true,
		adapters: []platform.Adapter{a},
	}

	err := runMarketplaceListWith(deps)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Copilot (1):")
	assert.NotContains(t, output, "Claude")
}

func TestRunMarketplaceListWith_OfficialSortsFirst(t *testing.T) {
	var buf bytes.Buffer
	a := newFakeAdapter("copilot")
	a.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return []platform.MarketplaceInfo{
			{Name: "zzz-marketplace", Source: "https://github.com/org/zzz"},
			{Name: "aaa-marketplace", Source: "https://github.com/org/aaa"},
			{Name: "summon-marketplace", Source: "https://github.com/ai-summon/summon-marketplace"},
		}, nil
	}

	deps := &marketplaceListDeps{
		stdout:   &buf,
		noColor:  true,
		adapters: []platform.Adapter{a},
	}

	err := runMarketplaceListWith(deps)
	require.NoError(t, err)

	output := buf.String()
	summonIdx := strings.Index(output, "summon-marketplace")
	aaaIdx := strings.Index(output, "aaa-marketplace")
	zzzIdx := strings.Index(output, "zzz-marketplace")
	assert.Less(t, summonIdx, aaaIdx, "official marketplace should appear before aaa")
	assert.Less(t, aaaIdx, zzzIdx, "aaa should appear before zzz")
	assert.Contains(t, output, "official")
}

func TestRunMarketplaceListWith_AdapterErrorSkipped(t *testing.T) {
	var buf bytes.Buffer
	failing := newFakeAdapter("copilot")
	failing.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return nil, fmt.Errorf("connection refused")
	}
	working := newFakeAdapter("claude")
	working.listMarketplacesFunc = func() ([]platform.MarketplaceInfo, error) {
		return []platform.MarketplaceInfo{
			{Name: "summon-marketplace", Source: "https://github.com/ai-summon/summon-marketplace"},
		}, nil
	}

	deps := &marketplaceListDeps{
		stdout:   &buf,
		noColor:  true,
		adapters: []platform.Adapter{failing, working},
	}

	err := runMarketplaceListWith(deps)
	require.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "Copilot")
	assert.Contains(t, output, "Claude (1):")
}

func TestRunMarketplaceListWith_NoAdapters(t *testing.T) {
	var buf bytes.Buffer
	deps := &marketplaceListDeps{
		stdout:   &buf,
		adapters: nil,
	}

	err := runMarketplaceListWith(deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs detected")
}

// --- Remove tests ---

func TestRunMarketplaceRemoveWith_DelegatesToAdapters(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	var removed []string
	a1 := newFakeAdapter("copilot")
	a1.removeMarketplaceFunc = func(name string) error {
		removed = append(removed, "copilot:"+name)
		return nil
	}
	a2 := newFakeAdapter("claude")
	a2.removeMarketplaceFunc = func(name string) error {
		removed = append(removed, "claude:"+name)
		return nil
	}

	deps := &removeDeps{
		stdout:   &outBuf,
		stderr:   &errBuf,
		adapters: []platform.Adapter{a1, a2},
	}

	err := runMarketplaceRemoveWith("my-mkt", deps)
	require.NoError(t, err)

	assert.Equal(t, []string{"copilot:my-mkt", "claude:my-mkt"}, removed)
	assert.Contains(t, outBuf.String(), "✓ copilot: marketplace removed")
	assert.Contains(t, outBuf.String(), "✓ claude: marketplace removed")
}

func TestRunMarketplaceRemoveWith_BlocksOfficialRemoval(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	deps := &removeDeps{
		stdout:   &outBuf,
		stderr:   &errBuf,
		adapters: []platform.Adapter{newFakeAdapter("copilot")},
	}

	err := runMarketplaceRemoveWith("summon-marketplace", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove the official marketplace")
}

func TestRunMarketplaceRemoveWith_NoAdapters(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	deps := &removeDeps{
		stdout:   &outBuf,
		stderr:   &errBuf,
		adapters: nil,
	}

	err := runMarketplaceRemoveWith("test", deps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no supported CLIs detected")
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
