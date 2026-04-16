package manifest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fake HTTP Client ---

type fakeHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func newFakeHTTPClient() *fakeHTTPClient {
	return &fakeHTTPClient{
		responses: make(map[string]*http.Response),
		errors:    make(map[string]error),
	}
}

func (f *fakeHTTPClient) Get(url string) (*http.Response, error) {
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

func (f *fakeHTTPClient) setResponse(url string, status int, body string) {
	f.responses[url] = &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// --- Fake Git Runner ---

type fakeGitRunner struct {
	runFunc func(name string, args ...string) ([]byte, error)
}

func (f *fakeGitRunner) Run(name string, args ...string) ([]byte, error) {
	if f.runFunc != nil {
		return f.runFunc(name, args...)
	}
	return nil, fmt.Errorf("not implemented")
}

// --- Tests ---

func TestFetchManifest_GitHubURL(t *testing.T) {
	httpClient := newFakeHTTPClient()
	manifestYAML := `
dependencies:
  - other-plugin
`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/owner/my-plugin/HEAD/summon.yaml",
		http.StatusOK,
		manifestYAML,
	)

	fetcher := NewRemoteFetcher(httpClient, &fakeGitRunner{})
	m, err := fetcher.FetchManifest("gh:owner/my-plugin")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Contains(t, m.Dependencies, "other-plugin")
}

func TestFetchManifest_GitHubURL_NoManifest(t *testing.T) {
	httpClient := newFakeHTTPClient()
	// 404 response (default for unknown URLs)

	fetcher := NewRemoteFetcher(httpClient, &fakeGitRunner{})
	m, err := fetcher.FetchManifest("gh:owner/no-manifest-plugin")
	require.NoError(t, err)
	assert.Nil(t, m)
}

func TestFetchManifest_NonGitHub_FallbackToGitArchive(t *testing.T) {
	httpClient := newFakeHTTPClient()
	manifestYAML := `dependencies:
  - some-dep
`
	gitRunner := &fakeGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(manifestYAML), nil
		},
	}

	fetcher := NewRemoteFetcher(httpClient, gitRunner)
	m, err := fetcher.FetchManifest("https://intranet.example.com/org/plugin.git")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Contains(t, m.Dependencies, "some-dep")
}

func TestFetchManifest_CacheHit(t *testing.T) {
	httpClient := newFakeHTTPClient()
	manifestYAML := `dependencies:
  - cached-dep
`
	httpClient.setResponse(
		"https://raw.githubusercontent.com/owner/cached-plugin/HEAD/summon.yaml",
		http.StatusOK,
		manifestYAML,
	)

	fetcher := NewRemoteFetcher(httpClient, &fakeGitRunner{})

	// First fetch
	m1, err := fetcher.FetchManifest("gh:owner/cached-plugin")
	require.NoError(t, err)
	require.NotNil(t, m1)

	// Second fetch should use cache (even if HTTP client would return different data)
	httpClient.setResponse(
		"https://raw.githubusercontent.com/owner/cached-plugin/HEAD/summon.yaml",
		http.StatusNotFound,
		"",
	)
	m2, err := fetcher.FetchManifest("gh:owner/cached-plugin")
	require.NoError(t, err)
	assert.Equal(t, m1.Dependencies, m2.Dependencies)
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
