package marketplace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-summon/summon/internal/git"
)

// PackageEntry represents a single entry in a marketplace index.
type PackageEntry struct {
	Source      string `json:"source"`
	Description string `json:"description"`
}

// MarketplaceJSON is the on-disk format of .claude-plugin/marketplace.json.
type MarketplaceJSON struct {
	Name     string               `json:"name"`
	Owner    MarketplaceOwner     `json:"owner"`
	Metadata MarketplaceMetadata  `json:"metadata"`
	Plugins  []MarketplacePlugin  `json:"plugins"`
}

// MarketplaceOwner identifies the marketplace owner.
type MarketplaceOwner struct {
	Name string `json:"name"`
}

// MarketplaceMetadata holds marketplace version info.
type MarketplaceMetadata struct {
	Description string `json:"description"`
	Version     string `json:"version"`
}

// MarketplacePlugin is a plugin entry in the marketplace JSON.
type MarketplacePlugin struct {
	Name        string                  `json:"name"`
	Source      MarketplacePluginSource `json:"source"`
	Description string                  `json:"description"`
}

// MarketplacePluginSource describes where a plugin lives.
// It handles all Claude Code marketplace source types:
//   - "github":    { "source": "github", "repo": "owner/repo" }
//   - "url":       { "source": "url", "url": "https://..." }
//   - "git-subdir":{ "source": "git-subdir", "url": "...", "path": "..." }
//   - "npm":       { "source": "npm", "package": "...", "version": "..." }
//   - string:      "./relative/path"
type MarketplacePluginSource struct {
	Source   string `json:"source"`            // source type: "github", "url", "git-subdir", "npm"
	Repo     string `json:"repo,omitempty"`    // for "github"
	URL      string `json:"url,omitempty"`     // for "url" and "git-subdir"
	Path     string `json:"path,omitempty"`    // for "git-subdir"
	Package  string `json:"package,omitempty"` // for "npm"
	Version  string `json:"version,omitempty"` // for "npm"
	Registry string `json:"registry,omitempty"`// for "npm"
	Ref      string `json:"ref,omitempty"`     // git ref (branch/tag)
	SHA      string `json:"sha,omitempty"`     // git commit sha
	Raw      string `json:"-"`                 // for plain string sources (relative paths)
}

// UnmarshalJSON handles the source field being either a string or an object.
func (s *MarketplacePluginSource) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		s.Raw = raw
		return nil
	}
	// Object form — use an alias to avoid recursion
	type Alias MarketplacePluginSource
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = MarketplacePluginSource(a)
	return nil
}

// resolvePluginSource converts a MarketplacePluginSource to a usable install source string.
func resolvePluginSource(s MarketplacePluginSource) (string, error) {
	if s.Raw != "" {
		return s.Raw, nil
	}
	switch s.Source {
	case "github":
		if s.Repo == "" {
			return "", fmt.Errorf("github source missing repo field")
		}
		return "gh:" + s.Repo, nil
	case "url":
		if s.URL == "" {
			return "", fmt.Errorf("url source missing url field")
		}
		return s.URL, nil
	case "git-subdir":
		if s.URL == "" {
			return "", fmt.Errorf("git-subdir source missing url field")
		}
		return s.URL, nil
	case "npm":
		if s.Package == "" {
			return "", fmt.Errorf("npm source missing package field")
		}
		if s.Version != "" {
			return "npm:" + s.Package + "@" + s.Version, nil
		}
		return "npm:" + s.Package, nil
	default:
		if s.Source == "" {
			return "", fmt.Errorf("source type is empty")
		}
		return "", fmt.Errorf("unknown source type %q", s.Source)
	}
}

// Index is the marketplace index: package name → entry.
type Index map[string]PackageEntry

// Registry provides marketplace package lookup.
type Registry struct {
	gitClient *git.Client
}

// NewRegistry creates a new Registry.
func NewRegistry(gitClient *git.Client) *Registry {
	return &Registry{gitClient: gitClient}
}

// FetchIndex parses a marketplace index from raw JSON data.
// Supports both the native .claude-plugin/marketplace.json format
// and the flat {name: {source, description}} format.
func FetchIndex(data []byte) (Index, error) {
	// Try native marketplace format first
	var mkt MarketplaceJSON
	if err := json.Unmarshal(data, &mkt); err == nil && len(mkt.Plugins) > 0 {
		idx := make(Index)
		for _, p := range mkt.Plugins {
			source, err := resolvePluginSource(p.Source)
			if err != nil {
				continue // skip plugins with unresolvable sources
			}
			idx[p.Name] = PackageEntry{
				Source:      source,
				Description: p.Description,
			}
		}
		return idx, nil
	}

	// Fall back to flat format
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse marketplace index: %w", err)
	}
	return idx, nil
}

// Lookup finds a package in the index by name.
func (idx Index) Lookup(name string) (*PackageEntry, bool) {
	entry, ok := idx[name]
	if !ok {
		return nil, false
	}
	return &entry, true
}

// OfficialMarketplaceURL is the default marketplace repo.
const OfficialMarketplaceURL = "https://github.com/ai-summon/summon-marketplace"

// MarketplaceSource represents a named marketplace source.
type MarketplaceSource struct {
	Name   string
	Source string
}

// HTTPClient abstracts HTTP requests for testing.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// GitRunner abstracts git command execution for testing.
type GitRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// IndexFetcher abstracts marketplace index fetching for dependency injection.
type IndexFetcher interface {
	FetchMarketplaceIndex(source string) (Index, error)
	LookupPackage(name string, marketplaceSource string) (*PackageEntry, error)
}

// DefaultIndexFetcher fetches marketplace indices via HTTP or git archive.
type DefaultIndexFetcher struct {
	httpClient HTTPClient
	gitRunner  GitRunner
	cache      map[string]Index
}

// NewDefaultIndexFetcher creates a new DefaultIndexFetcher.
func NewDefaultIndexFetcher(httpClient HTTPClient, gitRunner GitRunner) *DefaultIndexFetcher {
	return &DefaultIndexFetcher{
		httpClient: httpClient,
		gitRunner:  gitRunner,
		cache:      make(map[string]Index),
	}
}

// FetchMarketplaceIndex fetches a marketplace index from a git source, with caching.
func (f *DefaultIndexFetcher) FetchMarketplaceIndex(source string) (Index, error) {
	if idx, ok := f.cache[source]; ok {
		return idx, nil
	}

	idx, err := f.doFetch(source)
	if err != nil {
		return nil, err
	}
	f.cache[source] = idx
	return idx, nil
}

// LookupPackage fetches the marketplace index and looks up a package by name.
func (f *DefaultIndexFetcher) LookupPackage(name string, marketplaceSource string) (*PackageEntry, error) {
	idx, err := f.FetchMarketplaceIndex(marketplaceSource)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch marketplace index from %s: %w", marketplaceSource, err)
	}
	entry, ok := idx.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("package %q not found in marketplace %s", name, marketplaceSource)
	}
	return entry, nil
}

func (f *DefaultIndexFetcher) doFetch(source string) (Index, error) {
	if owner, repo, ok := parseGitHubURL(source); ok {
		return f.fetchFromGitHub(owner, repo)
	}
	return f.fetchViaGitArchive(source)
}

// fetchFromGitHub tries .claude-plugin/marketplace.json first, then marketplace.json at root.
func (f *DefaultIndexFetcher) fetchFromGitHub(owner, repo string) (Index, error) {
	paths := []string{
		".claude-plugin/marketplace.json",
		"marketplace.json",
	}

	for _, path := range paths {
		url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s", owner, repo, path)
		resp, err := f.httpClient.Get(url)
		if err != nil {
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusNotFound {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			continue
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read marketplace index body: %w", err)
		}
		return FetchIndex(data)
	}

	return nil, fmt.Errorf("marketplace index not found in %s/%s (tried .claude-plugin/marketplace.json and marketplace.json)", owner, repo)
}

func (f *DefaultIndexFetcher) fetchViaGitArchive(source string) (Index, error) {
	// Try .claude-plugin/marketplace.json first
	output, err := f.gitRunner.Run("git", "archive", "--remote="+source, "HEAD", ".claude-plugin/marketplace.json")
	if err != nil {
		// Fall back to marketplace.json at root
		output, err = f.gitRunner.Run("git", "archive", "--remote="+source, "HEAD", "marketplace.json")
		if err != nil {
			return nil, fmt.Errorf("failed to fetch marketplace index via git archive from %s: %w", source, err)
		}
	}
	return FetchIndex(output)
}

// parseGitHubURL extracts owner and repo from GitHub URLs or gh: shorthand.
func parseGitHubURL(source string) (owner, repo string, ok bool) {
	if strings.HasPrefix(source, "gh:") {
		parts := strings.SplitN(strings.TrimPrefix(source, "gh:"), "/", 2)
		if len(parts) == 2 {
			return parts[0], strings.TrimSuffix(parts[1], ".git"), true
		}
		return "", "", false
	}

	if strings.HasPrefix(source, "https://github.com/") {
		path := strings.TrimPrefix(source, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 3)
		if len(parts) >= 2 {
			return parts[0], parts[1], true
		}
	}

	return "", "", false
}

// ReadLocalIndex reads a marketplace index from the native CLI's local cache.
// It checks known cache paths where Claude Code and Copilot CLI store marketplace data.
// Returns the parsed Index or an error if no local cache is found.
func ReadLocalIndex(name string) (Index, error) {
	return readLocalIndexWithHome(name, "")
}

// ReadLocalIndexWithHome is the testable version of ReadLocalIndex that accepts a home directory.
func ReadLocalIndexWithHome(name string, homeDir string) (Index, error) {
	return readLocalIndexWithHome(name, homeDir)
}

// readLocalIndexWithHome is the internal implementation.
func readLocalIndexWithHome(name string, homeDir string) (Index, error) {
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
	}

	// Known local cache paths for marketplace indices
	paths := []string{
		// Claude Code stores marketplace indices here
		filepath.Join(homeDir, ".claude", "plugins", "marketplaces", name, ".claude-plugin", "marketplace.json"),
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		idx, err := FetchIndex(data)
		if err != nil {
			continue
		}
		return idx, nil
	}

	return nil, fmt.Errorf("no local cache found for marketplace %q", name)
}
