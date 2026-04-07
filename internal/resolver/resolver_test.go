package resolver

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSemver(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{"v1.0.0", true},
		{"1.2.3", true},
		{"v0.1.0-beta", true},
		{"v1.0", false},
		{"latest", false},
		{"main", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			assert.Equal(t, tt.want, isSemver(tt.tag))
		})
	}
}

func TestSortSemverTags(t *testing.T) {
	tags := []string{"v2.0.0", "v0.1.0", "v1.0.0", "v1.2.0", "v0.0.1"}
	sortSemverTags(tags)
	assert.Equal(t, []string{"v0.0.1", "v0.1.0", "v1.0.0", "v1.2.0", "v2.0.0"}, tags)
}

func TestSemverParts(t *testing.T) {
	major, minor, patch, ok := semverParts("v1.2.3")
	assert.True(t, ok)
	assert.Equal(t, 1, major)
	assert.Equal(t, 2, minor)
	assert.Equal(t, 3, patch)
}

func TestSemverParts_PreRelease(t *testing.T) {
	major, minor, patch, ok := semverParts("1.0.0-beta.1")
	assert.True(t, ok)
	assert.Equal(t, 1, major)
	assert.Equal(t, 0, minor)
	assert.Equal(t, 0, patch)
}

func TestSemverParts_Invalid(t *testing.T) {
	_, _, _, ok := semverParts("not-a-version")
	assert.False(t, ok)
}

// ── additional semverParts tests ─────────────────────────────────

func TestSemverParts_NoPrefix(t *testing.T) {
	major, minor, patch, ok := semverParts("1.2.3")
	assert.True(t, ok)
	assert.Equal(t, 1, major)
	assert.Equal(t, 2, minor)
	assert.Equal(t, 3, patch)
}

func TestSemverParts_TwoParts(t *testing.T) {
	_, _, _, ok := semverParts("1.2")
	assert.False(t, ok)
}

// ── additional sortSemverTags tests ──────────────────────────────

func TestSortSemverTags_PatchLevel(t *testing.T) {
	tags := []string{"v1.0.3", "v1.0.1", "v1.0.2"}
	sortSemverTags(tags)
	assert.Equal(t, []string{"v1.0.1", "v1.0.2", "v1.0.3"}, tags)
}

// ── ResolveLatest tests (use local git repos) ────────────────────

// initTaggedRepo creates a git repo with one commit and the given tags.
func initTaggedRepo(t *testing.T, tags ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	commit := exec.Command("git", "commit", "--allow-empty", "-m", "init")
	commit.Dir = dir
	out, err := commit.CombinedOutput()
	require.NoError(t, err, string(out))

	for _, tag := range tags {
		cmd := exec.Command("git", "tag", tag)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

func TestResolveLatest_SemverTags(t *testing.T) {
	repo := initTaggedRepo(t, "v1.0.0", "v2.0.0", "v1.5.0")
	tag, err := ResolveLatest(repo)
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", tag)
}

func TestResolveLatest_NoTags(t *testing.T) {
	repo := initTaggedRepo(t)
	tag, err := ResolveLatest(repo)
	require.NoError(t, err)
	assert.Equal(t, "HEAD", tag)
}

func TestResolveLatest_NonSemverOnly(t *testing.T) {
	repo := initTaggedRepo(t, "latest", "nightly", "stable")
	tag, err := ResolveLatest(repo)
	require.NoError(t, err)
	assert.Equal(t, "HEAD", tag)
}
