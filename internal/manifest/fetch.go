package manifest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ManifestFetcher abstracts remote manifest fetching.
type ManifestFetcher interface {
	FetchManifest(source string) (*Manifest, error)
}

// HTTPClient abstracts HTTP requests for testing.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// GitRunner abstracts git command execution for testing.
type GitRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// RemoteFetcher fetches manifests from remote repositories.
type RemoteFetcher struct {
	httpClient HTTPClient
	gitRunner  GitRunner
	cache      map[string]*fetchResult
}

type fetchResult struct {
	manifest *Manifest
	err      error
	fetched  bool
}

// NewRemoteFetcher creates a new RemoteFetcher.
func NewRemoteFetcher(httpClient HTTPClient, gitRunner GitRunner) *RemoteFetcher {
	return &RemoteFetcher{
		httpClient: httpClient,
		gitRunner:  gitRunner,
		cache:      make(map[string]*fetchResult),
	}
}

// FetchManifest fetches and parses a summon.yaml from a remote source.
// Returns nil (no error) if the repo exists but has no summon.yaml.
func (f *RemoteFetcher) FetchManifest(source string) (*Manifest, error) {
	if cached, ok := f.cache[source]; ok {
		return cached.manifest, cached.err
	}

	manifest, err := f.doFetch(source)
	f.cache[source] = &fetchResult{manifest: manifest, err: err, fetched: true}
	return manifest, err
}

func (f *RemoteFetcher) doFetch(source string) (*Manifest, error) {
	// Try GitHub raw content API for GitHub URLs
	if owner, repo, ok := parseGitHubURL(source); ok {
		return f.fetchFromGitHub(owner, repo)
	}

	// Fallback: try git archive
	return f.fetchViaGitArchive(source)
}

func (f *RemoteFetcher) fetchFromGitHub(owner, repo string) (*Manifest, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/summon.yaml", owner, repo)

	resp, err := f.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest from GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No manifest — valid scenario
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub raw content returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	return ParseAndValidate(data)
}

func (f *RemoteFetcher) fetchViaGitArchive(source string) (*Manifest, error) {
	output, err := f.gitRunner.Run("git", "archive", "--remote="+source, "HEAD", "summon.yaml")
	if err != nil {
		// If archive fails, the repo may not have summon.yaml or archive isn't supported
		return nil, nil
	}

	return ParseAndValidate(output)
}

// parseGitHubURL extracts owner and repo from GitHub URLs or gh: shorthand.
func parseGitHubURL(source string) (owner, repo string, ok bool) {
	// Handle gh:owner/repo shorthand
	if strings.HasPrefix(source, "gh:") {
		parts := strings.SplitN(strings.TrimPrefix(source, "gh:"), "/", 2)
		if len(parts) == 2 {
			return parts[0], strings.TrimSuffix(parts[1], ".git"), true
		}
		return "", "", false
	}

	// Handle https://github.com/owner/repo URLs
	if strings.HasPrefix(source, "https://github.com/") {
		path := strings.TrimPrefix(source, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 3) // limit to 3 to handle paths like owner/repo/tree/...
		if len(parts) >= 2 {
			return parts[0], parts[1], true
		}
	}

	return "", "", false
}
