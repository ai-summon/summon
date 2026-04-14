package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_BareName(t *testing.T) {
	r, err := Resolve("my-plugin")
	require.NoError(t, err)
	assert.Equal(t, SourceOfficialMarketplace, r.Type)
	assert.Equal(t, "my-plugin", r.Name)
}

func TestResolve_NameAtMarketplace(t *testing.T) {
	r, err := Resolve("cool-tool@my-marketplace")
	require.NoError(t, err)
	assert.Equal(t, SourceNamedMarketplace, r.Type)
	assert.Equal(t, "cool-tool", r.Name)
	assert.Equal(t, "my-marketplace", r.MarketplaceName)
}

func TestResolve_GitHubShorthand(t *testing.T) {
	r, err := Resolve("gh:owner/repo")
	require.NoError(t, err)
	assert.Equal(t, SourceGitHubShorthand, r.Type)
	assert.Equal(t, "repo", r.Name)
	assert.Equal(t, "https://github.com/owner/repo", r.Source)
}

func TestResolve_FullURL(t *testing.T) {
	r, err := Resolve("https://github.com/owner/my-plugin")
	require.NoError(t, err)
	assert.Equal(t, SourceDirectURL, r.Type)
	assert.Equal(t, "my-plugin", r.Name)
	assert.Equal(t, "https://github.com/owner/my-plugin", r.Source)
}

func TestResolve_FullURLWithGit(t *testing.T) {
	r, err := Resolve("https://intranet.example.com/org/plugin.git")
	require.NoError(t, err)
	assert.Equal(t, SourceDirectURL, r.Type)
	assert.Equal(t, "plugin", r.Name)
}

func TestResolve_InvalidEmpty(t *testing.T) {
	_, err := Resolve("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestResolve_InvalidFormat(t *testing.T) {
	_, err := Resolve("gh:invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected gh:owner/repo")
}

func TestResolve_InvalidBareName(t *testing.T) {
	_, err := Resolve("My_Plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kebab-case")
}

func TestResolve_InvalidMarketplaceRef(t *testing.T) {
	_, err := Resolve("@marketplace")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected name@marketplace")
}

func TestResolve_GitProtocol(t *testing.T) {
	r, err := Resolve("git://example.com/repo.git")
	require.NoError(t, err)
	assert.Equal(t, SourceDirectURL, r.Type)
	assert.Equal(t, "repo", r.Name)
}
