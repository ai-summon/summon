package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/resolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- confirmPromptDefault tests ---

func TestConfirmPromptDefault_EmptyInputDefaultYes(t *testing.T) {
	reader := strings.NewReader("\n")
	assert.True(t, confirmPromptDefault(reader, true))
}

func TestConfirmPromptDefault_EmptyInputDefaultNo(t *testing.T) {
	reader := strings.NewReader("\n")
	assert.False(t, confirmPromptDefault(reader, false))
}

func TestConfirmPromptDefault_ExplicitY(t *testing.T) {
	reader := strings.NewReader("y\n")
	assert.True(t, confirmPromptDefault(reader, false))
}

func TestConfirmPromptDefault_ExplicitYes(t *testing.T) {
	reader := strings.NewReader("yes\n")
	assert.True(t, confirmPromptDefault(reader, false))
}

func TestConfirmPromptDefault_ExplicitNo(t *testing.T) {
	reader := strings.NewReader("n\n")
	assert.False(t, confirmPromptDefault(reader, true))
}

func TestConfirmPromptDefault_ExplicitN(t *testing.T) {
	reader := strings.NewReader("no\n")
	assert.False(t, confirmPromptDefault(reader, true))
}

func TestConfirmPromptDefault_GarbageInput(t *testing.T) {
	reader := strings.NewReader("maybe\n")
	assert.False(t, confirmPromptDefault(reader, true))
}

func TestConfirmPromptDefault_NoInput_DefaultYes(t *testing.T) {
	reader := strings.NewReader("") // EOF, no scan
	assert.True(t, confirmPromptDefault(reader, true))
}

func TestConfirmPromptDefault_NoInput_DefaultNo(t *testing.T) {
	reader := strings.NewReader("") // EOF, no scan
	assert.False(t, confirmPromptDefault(reader, false))
}

func TestConfirmPromptDefault_CaseInsensitive(t *testing.T) {
	reader := strings.NewReader("YES\n")
	assert.True(t, confirmPromptDefault(reader, false))
}

func TestConfirmPromptDefault_WhitespaceY(t *testing.T) {
	reader := strings.NewReader("  y  \n")
	assert.True(t, confirmPromptDefault(reader, false))
}

// --- resolveInstallSource tests ---

func TestResolveInstallSource_OfficialMarketplace(t *testing.T) {
	resolved := &resolver.ResolvedSource{
		Type: resolver.SourceOfficialMarketplace,
		Name: "my-plugin",
	}
	source, err := resolveInstallSource(resolved)
	require.NoError(t, err)
	assert.Equal(t, "my-plugin@summon-marketplace", source)
}

func TestResolveInstallSource_NamedMarketplace(t *testing.T) {
	resolved := &resolver.ResolvedSource{
		Type:            resolver.SourceNamedMarketplace,
		Name:            "my-plugin",
		MarketplaceName: "custom-mkt",
	}
	source, err := resolveInstallSource(resolved)
	require.NoError(t, err)
	assert.Equal(t, "my-plugin@custom-mkt", source)
}

func TestResolveInstallSource_GitHubShorthand(t *testing.T) {
	resolved := &resolver.ResolvedSource{
		Type:   resolver.SourceGitHubShorthand,
		Name:   "my-plugin",
		Source: "https://github.com/owner/my-plugin",
	}
	source, err := resolveInstallSource(resolved)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/my-plugin", source)
}

func TestResolveInstallSource_DirectURL(t *testing.T) {
	resolved := &resolver.ResolvedSource{
		Type:   resolver.SourceDirectURL,
		Name:   "my-plugin",
		Source: "https://example.com/plugins/my-plugin.git",
	}
	source, err := resolveInstallSource(resolved)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/plugins/my-plugin.git", source)
}

func TestResolveInstallSource_NativeMarketplace_WithSource(t *testing.T) {
	resolved := &resolver.ResolvedSource{
		Type:   resolver.SourceNativeMarketplace,
		Name:   "native-pkg",
		Source: "native-source-ref",
	}
	source, err := resolveInstallSource(resolved)
	require.NoError(t, err)
	assert.Equal(t, "native-source-ref", source)
}

func TestResolveInstallSource_NativeMarketplace_NoSource(t *testing.T) {
	resolved := &resolver.ResolvedSource{
		Type: resolver.SourceNativeMarketplace,
		Name: "native-pkg",
	}
	source, err := resolveInstallSource(resolved)
	require.NoError(t, err)
	assert.Equal(t, "native-pkg", source)
}

// --- getOrCacheManifest tests ---

func TestGetOrCacheManifest_ReturnsCachedManifest(t *testing.T) {
	cached := &manifest.Manifest{Dependencies: []string{"dep-a"}}
	cache := map[string]*manifest.Manifest{"my-pkg": cached}
	adapter := newFakeAdapter("claude")
	fetcher := newFakeFetcher()
	var stderr bytes.Buffer

	result := getOrCacheManifest("my-pkg", cache, adapter, platform.ScopeUser, fetcher, &stderr)
	assert.Equal(t, cached, result)
}

func TestGetOrCacheManifest_FindPluginDirError(t *testing.T) {
	cache := make(map[string]*manifest.Manifest)
	adapter := newFakeAdapter("claude")
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "", fmt.Errorf("plugin dir not found")
	}
	fetcher := newFakeFetcher()
	var stderr bytes.Buffer

	result := getOrCacheManifest("missing-pkg", cache, adapter, platform.ScopeUser, fetcher, &stderr)
	assert.Nil(t, result)
	assert.Empty(t, stderr.String()) // no warning printed for FindPluginDir errors
}

func TestGetOrCacheManifest_FetchManifestError(t *testing.T) {
	cache := make(map[string]*manifest.Manifest)
	adapter := newFakeAdapter("claude")
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}

	errFetcher := &errorFetcher{err: fmt.Errorf("corrupt manifest")}
	var stderr bytes.Buffer

	result := getOrCacheManifest("bad-pkg", cache, adapter, platform.ScopeUser, errFetcher, &stderr)
	assert.Nil(t, result)
	assert.Contains(t, stderr.String(), "Warning: failed to read manifest for bad-pkg")
	assert.Contains(t, stderr.String(), "corrupt manifest")
}

func TestGetOrCacheManifest_FetchReturnsNil(t *testing.T) {
	cache := make(map[string]*manifest.Manifest)
	adapter := newFakeAdapter("claude")
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}
	fetcher := newFakeFetcher() // no manifests registered → returns nil, nil
	var stderr bytes.Buffer

	result := getOrCacheManifest("no-manifest-pkg", cache, adapter, platform.ScopeUser, fetcher, &stderr)
	assert.Nil(t, result)
	// nil manifest should not be cached
	_, cached := cache["no-manifest-pkg"]
	assert.False(t, cached)
}

func TestGetOrCacheManifest_CachesSuccessfulFetch(t *testing.T) {
	cache := make(map[string]*manifest.Manifest)
	adapter := newFakeAdapter("claude")
	adapter.findDirFunc = func(name string, scope platform.Scope) (string, error) {
		return "/fake/plugins/" + name, nil
	}
	fetcher := newFakeFetcher()
	fetcher.manifests["/fake/plugins/cached-pkg"] = &manifest.Manifest{
		Dependencies: []string{"dep-x"},
	}
	var stderr bytes.Buffer

	result := getOrCacheManifest("cached-pkg", cache, adapter, platform.ScopeUser, fetcher, &stderr)
	require.NotNil(t, result)
	assert.Equal(t, []string{"dep-x"}, result.Dependencies)

	// Verify it was cached
	cachedM, ok := cache["cached-pkg"]
	assert.True(t, ok)
	assert.Equal(t, result, cachedM)
}

// --- deriveMarketplaceName tests ---

func TestDeriveMarketplaceName_FullURL(t *testing.T) {
	assert.Equal(t, "my-marketplace", deriveMarketplaceName("https://github.com/org/my-marketplace"))
}

func TestDeriveMarketplaceName_URLWithGitSuffix(t *testing.T) {
	assert.Equal(t, "my-marketplace", deriveMarketplaceName("https://github.com/org/my-marketplace.git"))
}

func TestDeriveMarketplaceName_TrailingSlash(t *testing.T) {
	assert.Equal(t, "my-marketplace", deriveMarketplaceName("https://github.com/org/my-marketplace/"))
}

func TestDeriveMarketplaceName_TrailingSlashAndGit(t *testing.T) {
	// TrimSuffix(".git") doesn't match when followed by "/", so .git remains in the last segment
	assert.Equal(t, "my-marketplace.git", deriveMarketplaceName("https://github.com/org/my-marketplace.git/"))
}

func TestDeriveMarketplaceName_SimpleName(t *testing.T) {
	assert.Equal(t, "solo", deriveMarketplaceName("solo"))
}

func TestDeriveMarketplaceName_EmptyString(t *testing.T) {
	// empty after trimming → falls through, returns empty string
	result := deriveMarketplaceName("")
	assert.Equal(t, "", result)
}

func TestDeriveMarketplaceName_OnlySlashes(t *testing.T) {
	// After TrimRight("/", "/") we get empty, Split returns [""]
	result := deriveMarketplaceName("///")
	assert.Equal(t, "", result)
}

func TestDeriveMarketplaceName_DotGitOnly(t *testing.T) {
	result := deriveMarketplaceName(".git")
	assert.Equal(t, "", result)
}

// --- Install with named marketplace ---

func TestInstall_NamedMarketplace(t *testing.T) {
	adapter := newFakeAdapter("claude")
	deps := newTestDeps(newFakeRunner(), newFakeFetcher(), []platform.Adapter{adapter}, "")
	installYes = true
	installForce = false
	installScope = "user"
	targetFlag = ""

	err := runInstall("my-tool@custom-mkt", deps)
	require.NoError(t, err)

	require.Len(t, adapter.installedCmds, 1)
	assert.Equal(t, "my-tool@custom-mkt", adapter.installedCmds[0])
}

// --- errorFetcher: returns an error on FetchManifest ---

type errorFetcher struct {
	err error
}

func (f *errorFetcher) FetchManifest(source string) (*manifest.Manifest, error) {
	return nil, f.err
}
