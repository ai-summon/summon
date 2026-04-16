package marketplace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fake HTTP Client for registry tests ---

type fakeRegistryHTTPClient struct {
	responses map[string]*http.Response
}

func newFakeRegistryHTTPClient() *fakeRegistryHTTPClient {
	return &fakeRegistryHTTPClient{
		responses: make(map[string]*http.Response),
	}
}

func (f *fakeRegistryHTTPClient) Get(url string) (*http.Response, error) {
	if resp, ok := f.responses[url]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (f *fakeRegistryHTTPClient) setResponse(url string, status int, body string) {
	f.responses[url] = &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// --- Fake Git Runner ---

type fakeRegistryGitRunner struct {
	runFunc func(name string, args ...string) ([]byte, error)
}

func (f *fakeRegistryGitRunner) Run(name string, args ...string) ([]byte, error) {
	if f.runFunc != nil {
		return f.runFunc(name, args...)
	}
	return nil, fmt.Errorf("not implemented")
}

// --- FetchIndex tests ---

func TestFetchIndex_NativeFormat(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"metadata": {"description": "Test marketplace", "version": "0.1.0"},
		"plugins": [
			{
				"name": "my-plugin",
				"source": {"source": "github", "repo": "owner/my-plugin"},
				"description": "A useful plugin"
			},
			{
				"name": "other-tool",
				"source": {"source": "github", "repo": "owner/other-tool"},
				"description": "Another tool"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 2)

	entry, found := idx.Lookup("my-plugin")
	assert.True(t, found)
	assert.Equal(t, "gh:owner/my-plugin", entry.Source)
}

func TestFetchIndex_URLSourceType(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "ghe-plugin",
				"source": {"source": "url", "url": "https://ghe.example.com/org/plugin.git"},
				"description": "A GHE-hosted plugin"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("ghe-plugin")
	assert.True(t, found)
	assert.Equal(t, "https://ghe.example.com/org/plugin.git", entry.Source)
}

func TestFetchIndex_GitSubdirSourceType(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "mono-plugin",
				"source": {"source": "git-subdir", "url": "https://github.com/acme/monorepo.git", "path": "tools/plugin", "ref": "main"},
				"description": "Plugin in a monorepo"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("mono-plugin")
	assert.True(t, found)
	assert.Equal(t, "https://github.com/acme/monorepo.git", entry.Source)
}

func TestFetchIndex_NpmSourceType(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "npm-plugin",
				"source": {"source": "npm", "package": "@acme/claude-plugin", "version": "2.1.0"},
				"description": "An npm plugin"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("npm-plugin")
	assert.True(t, found)
	assert.Equal(t, "npm:@acme/claude-plugin@2.1.0", entry.Source)
}

func TestFetchIndex_NpmSourceNoVersion(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "npm-plugin",
				"source": {"source": "npm", "package": "@acme/claude-plugin"},
				"description": "An npm plugin without version"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)

	entry, found := idx.Lookup("npm-plugin")
	assert.True(t, found)
	assert.Equal(t, "npm:@acme/claude-plugin", entry.Source)
}

func TestFetchIndex_RelativePathSource(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "local-plugin",
				"source": "./plugins/my-plugin",
				"description": "A local plugin"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("local-plugin")
	assert.True(t, found)
	assert.Equal(t, "./plugins/my-plugin", entry.Source)
}

func TestFetchIndex_MixedSourceTypes(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "gh-plugin",
				"source": {"source": "github", "repo": "owner/gh-plugin"},
				"description": "GitHub plugin"
			},
			{
				"name": "url-plugin",
				"source": {"source": "url", "url": "https://ghe.example.com/org/plugin.git"},
				"description": "URL plugin"
			},
			{
				"name": "local-plugin",
				"source": "./plugins/local",
				"description": "Local plugin"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 3)

	gh, _ := idx.Lookup("gh-plugin")
	assert.Equal(t, "gh:owner/gh-plugin", gh.Source)

	url, _ := idx.Lookup("url-plugin")
	assert.Equal(t, "https://ghe.example.com/org/plugin.git", url.Source)

	local, _ := idx.Lookup("local-plugin")
	assert.Equal(t, "./plugins/local", local.Source)
}

func TestFetchIndex_SkipsInvalidSource(t *testing.T) {
	data := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"plugins": [
			{
				"name": "good-plugin",
				"source": {"source": "github", "repo": "owner/good"},
				"description": "Good"
			},
			{
				"name": "bad-plugin",
				"source": {"source": "github"},
				"description": "Missing repo field"
			}
		]
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	_, found := idx.Lookup("good-plugin")
	assert.True(t, found)

	_, found = idx.Lookup("bad-plugin")
	assert.False(t, found)
}

func TestFetchIndex_FlatFormat(t *testing.T) {
	data := `{
		"my-plugin": {
			"source": "https://github.com/owner/my-plugin",
			"description": "A useful plugin"
		}
	}`
	idx, err := FetchIndex([]byte(data))
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("my-plugin")
	assert.True(t, found)
	assert.Equal(t, "https://github.com/owner/my-plugin", entry.Source)
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

// --- DefaultIndexFetcher tests ---

func TestDefaultIndexFetcher_FetchFromGitHub(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	indexJSON := `{
		"name": "summon-marketplace",
		"owner": {"name": "Summon"},
		"metadata": {"description": "Test", "version": "0.1.0"},
		"plugins": [
			{
				"name": "superpowers",
				"source": {"source": "github", "repo": "owner/superpowers"},
				"description": "A superpower plugin"
			}
		]
	}`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		http.StatusOK,
		indexJSON,
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	idx, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("superpowers")
	assert.True(t, found)
	assert.Equal(t, "gh:owner/superpowers", entry.Source)
}

func TestDefaultIndexFetcher_FallbackToRootMarketplaceJSON(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	// .claude-plugin/marketplace.json returns 404
	// marketplace.json at root returns data
	indexJSON := `{
		"name": "legacy-marketplace",
		"owner": {"name": "Test"},
		"metadata": {"description": "Legacy", "version": "0.1.0"},
		"plugins": [
			{
				"name": "legacy-tool",
				"source": {"source": "github", "repo": "owner/legacy-tool"},
				"description": "Legacy tool"
			}
		]
	}`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/marketplace.json",
		http.StatusOK,
		indexJSON,
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	idx, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	require.NoError(t, err)

	entry, found := idx.Lookup("legacy-tool")
	assert.True(t, found)
	assert.Equal(t, "gh:owner/legacy-tool", entry.Source)
}

func TestDefaultIndexFetcher_Cache(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	indexJSON := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"metadata": {"description": "Test", "version": "0.1.0"},
		"plugins": [
			{
				"name": "cached-pkg",
				"source": {"source": "github", "repo": "owner/cached-pkg"},
				"description": "Cached"
			}
		]
	}`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		http.StatusOK,
		indexJSON,
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})

	// First fetch
	idx1, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	require.NoError(t, err)
	assert.Len(t, idx1, 1)

	// Replace HTTP response with 404 — should still return cached
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		http.StatusNotFound,
		"",
	)
	idx2, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	require.NoError(t, err)
	assert.Equal(t, idx1, idx2)
}

func TestDefaultIndexFetcher_GitArchiveFallback(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	indexJSON := `{"internal-tool": {"source": "https://intranet.example.com/org/tool", "description": "Internal"}}`
	gitRunner := &fakeRegistryGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(indexJSON), nil
		},
	}

	fetcher := NewDefaultIndexFetcher(httpClient, gitRunner)
	idx, err := fetcher.FetchMarketplaceIndex("https://intranet.example.com/org/marketplace")
	require.NoError(t, err)

	entry, found := idx.Lookup("internal-tool")
	assert.True(t, found)
	assert.Equal(t, "https://intranet.example.com/org/tool", entry.Source)
}

func TestDefaultIndexFetcher_NotFound(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	// Default 404 response for both paths

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	_, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marketplace index not found")
}

func TestDefaultIndexFetcher_LookupPackage_Found(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	indexJSON := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"metadata": {"description": "Test", "version": "0.1.0"},
		"plugins": [
			{
				"name": "superpowers",
				"source": {"source": "github", "repo": "owner/superpowers"},
				"description": "Powers"
			}
		]
	}`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		http.StatusOK,
		indexJSON,
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	entry, err := fetcher.LookupPackage("superpowers", OfficialMarketplaceURL)
	require.NoError(t, err)
	assert.Equal(t, "gh:owner/superpowers", entry.Source)
}

func TestDefaultIndexFetcher_LookupPackage_NotFound(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	indexJSON := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"metadata": {"description": "Test", "version": "0.1.0"},
		"plugins": [
			{
				"name": "other-pkg",
				"source": {"source": "github", "repo": "owner/other"},
				"description": "Other"
			}
		]
	}`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		http.StatusOK,
		indexJSON,
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	_, err := fetcher.LookupPackage("nonexistent", OfficialMarketplaceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `package "nonexistent" not found`)
}

func TestParseGitHubURL_Shorthand(t *testing.T) {
	owner, repo, ok := parseGitHubURL("gh:owner/repo")
	assert.True(t, ok)
	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)
}

func TestParseGitHubURL_FullURL(t *testing.T) {
	owner, repo, ok := parseGitHubURL("https://github.com/owner/repo")
	assert.True(t, ok)
	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)
}

func TestParseGitHubURL_FullURLWithGit(t *testing.T) {
	owner, repo, ok := parseGitHubURL("https://github.com/owner/repo.git")
	assert.True(t, ok)
	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)
}

func TestParseGitHubURL_NonGitHub(t *testing.T) {
	_, _, ok := parseGitHubURL("https://intranet.example.com/org/repo")
	assert.False(t, ok)
}
