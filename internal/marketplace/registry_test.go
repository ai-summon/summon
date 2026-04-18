package marketplace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fake HTTP Client for registry tests ---

type fakeRegistryHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func newFakeRegistryHTTPClient() *fakeRegistryHTTPClient {
	return &fakeRegistryHTTPClient{
		responses: make(map[string]*http.Response),
		errors:    make(map[string]error),
	}
}

func (f *fakeRegistryHTTPClient) Get(url string) (*http.Response, error) {
	if err, ok := f.errors[url]; ok {
		return nil, err
	}
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

func (f *fakeRegistryHTTPClient) setError(url string, err error) {
	f.errors[url] = err
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

func TestParseGitHubURL_ShorthandNoSlash(t *testing.T) {
	_, _, ok := parseGitHubURL("gh:noslash")
	assert.False(t, ok)
}

// --- NewRegistry tests ---

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry(nil)
	require.NotNil(t, reg)
	assert.Nil(t, reg.gitClient)
}

// --- resolvePluginSource error path tests ---

func TestResolvePluginSource_EmptySourceType(t *testing.T) {
	s := MarketplacePluginSource{Source: ""}
	_, err := resolvePluginSource(s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source type is empty")
}

func TestResolvePluginSource_UnknownSourceType(t *testing.T) {
	s := MarketplacePluginSource{Source: "ftp"}
	_, err := resolvePluginSource(s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `unknown source type "ftp"`)
}

func TestResolvePluginSource_URLMissingURL(t *testing.T) {
	s := MarketplacePluginSource{Source: "url"}
	_, err := resolvePluginSource(s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url source missing url field")
}

func TestResolvePluginSource_GitSubdirMissingURL(t *testing.T) {
	s := MarketplacePluginSource{Source: "git-subdir"}
	_, err := resolvePluginSource(s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git-subdir source missing url field")
}

func TestResolvePluginSource_NpmMissingPackage(t *testing.T) {
	s := MarketplacePluginSource{Source: "npm"}
	_, err := resolvePluginSource(s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "npm source missing package field")
}

// --- UnmarshalJSON edge cases ---

func TestUnmarshalJSON_EmptyData(t *testing.T) {
	var s MarketplacePluginSource
	err := s.UnmarshalJSON([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, MarketplacePluginSource{}, s)
}

func TestUnmarshalJSON_InvalidString(t *testing.T) {
	var s MarketplacePluginSource
	err := s.UnmarshalJSON([]byte(`"unterminated`))
	assert.Error(t, err)
}

func TestUnmarshalJSON_InvalidObject(t *testing.T) {
	var s MarketplacePluginSource
	err := s.UnmarshalJSON([]byte(`{"source": 123}`))
	assert.Error(t, err)
}

// --- fetchFromGitHub error path tests ---

func TestDefaultIndexFetcher_FetchFromGitHub_HTTPError(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	// Both paths return HTTP errors
	httpClient.setError(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		fmt.Errorf("connection refused"),
	)
	httpClient.setError(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/marketplace.json",
		fmt.Errorf("connection refused"),
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	_, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marketplace index not found")
}

func TestDefaultIndexFetcher_FetchFromGitHub_NonOKNon404Status(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		http.StatusInternalServerError,
		"server error",
	)
	httpClient.setResponse(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/marketplace.json",
		http.StatusInternalServerError,
		"server error",
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	_, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marketplace index not found")
}

func TestDefaultIndexFetcher_FetchFromGitHub_ReadBodyError(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	httpClient.responses["https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json"] = &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&errReader{}),
	}

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	_, err := fetcher.FetchMarketplaceIndex(OfficialMarketplaceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read marketplace index body")
}

// errReader is an io.Reader that always returns an error.
type errReader struct{}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("read error")
}

// --- fetchViaGitArchive tests ---

func TestDefaultIndexFetcher_GitArchive_FirstFailsSecondSucceeds(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	indexJSON := `{"tool": {"source": "https://example.com/tool", "description": "A tool"}}`
	callCount := 0
	gitRunner := &fakeRegistryGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			callCount++
			// First call (for .claude-plugin/marketplace.json) fails
			if callCount == 1 {
				return nil, fmt.Errorf("file not found")
			}
			// Second call (for marketplace.json) succeeds
			return []byte(indexJSON), nil
		},
	}

	fetcher := NewDefaultIndexFetcher(httpClient, gitRunner)
	idx, err := fetcher.FetchMarketplaceIndex("https://intranet.example.com/org/marketplace")
	require.NoError(t, err)

	entry, found := idx.Lookup("tool")
	assert.True(t, found)
	assert.Equal(t, "https://example.com/tool", entry.Source)
}

func TestDefaultIndexFetcher_GitArchive_BothFail(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	gitRunner := &fakeRegistryGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("git archive failed")
		},
	}

	fetcher := NewDefaultIndexFetcher(httpClient, gitRunner)
	_, err := fetcher.FetchMarketplaceIndex("https://intranet.example.com/org/marketplace")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch marketplace index via git archive")
}

// --- LookupPackage error path ---

func TestDefaultIndexFetcher_LookupPackage_FetchError(t *testing.T) {
	httpClient := newFakeRegistryHTTPClient()
	httpClient.setError(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/.claude-plugin/marketplace.json",
		fmt.Errorf("network error"),
	)
	httpClient.setError(
		"https://raw.githubusercontent.com/ai-summon/summon-marketplace/HEAD/marketplace.json",
		fmt.Errorf("network error"),
	)

	fetcher := NewDefaultIndexFetcher(httpClient, &fakeRegistryGitRunner{})
	_, err := fetcher.LookupPackage("any-package", OfficialMarketplaceURL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch marketplace index")
}

// --- ReadLocalIndexWithHome tests ---

func TestReadLocalIndexWithHome_ValidIndex(t *testing.T) {
	homeDir := t.TempDir()
	dir := filepath.Join(homeDir, ".claude", "plugins", "marketplaces", "test-market", ".claude-plugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	indexJSON := `{
		"name": "test-marketplace",
		"owner": {"name": "Test"},
		"metadata": {"description": "Test", "version": "0.1.0"},
		"plugins": [
			{
				"name": "local-plugin",
				"source": {"source": "github", "repo": "owner/local-plugin"},
				"description": "A local plugin"
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(indexJSON), 0o644))

	idx, err := ReadLocalIndexWithHome("test-market", homeDir)
	require.NoError(t, err)
	assert.Len(t, idx, 1)

	entry, found := idx.Lookup("local-plugin")
	assert.True(t, found)
	assert.Equal(t, "gh:owner/local-plugin", entry.Source)
}

func TestReadLocalIndexWithHome_NoCache(t *testing.T) {
	homeDir := t.TempDir()

	_, err := ReadLocalIndexWithHome("nonexistent-market", homeDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no local cache found")
}

func TestReadLocalIndexWithHome_InvalidJSON(t *testing.T) {
	homeDir := t.TempDir()
	dir := filepath.Join(homeDir, ".claude", "plugins", "marketplaces", "bad-market", ".claude-plugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte("{invalid json"), 0o644))

	_, err := ReadLocalIndexWithHome("bad-market", homeDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no local cache found")
}

func TestReadLocalIndexWithHome_FlatFormat(t *testing.T) {
	homeDir := t.TempDir()
	dir := filepath.Join(homeDir, ".claude", "plugins", "marketplaces", "flat-market", ".claude-plugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	indexJSON := `{"my-tool": {"source": "https://github.com/org/tool", "description": "A tool"}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(indexJSON), 0o644))

	idx, err := ReadLocalIndexWithHome("flat-market", homeDir)
	require.NoError(t, err)

	entry, found := idx.Lookup("my-tool")
	assert.True(t, found)
	assert.Equal(t, "https://github.com/org/tool", entry.Source)
}

func TestReadLocalIndex_UsesUserHomeDir(t *testing.T) {
	// ReadLocalIndex delegates to readLocalIndexWithHome with empty homeDir.
	// This will use os.UserHomeDir(). We just verify it doesn't panic and
	// returns a reasonable error for a nonexistent marketplace name.
	_, err := ReadLocalIndex("nonexistent-marketplace-for-test-12345")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no local cache found")
}
