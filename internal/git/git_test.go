package git

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo creates a new git repo in dir with one committed file.
// It returns the path to the repo.
func initRepo(t *testing.T, dir string) string {
	t.Helper()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	// create a file and commit
	require.NoError(t, exec.Command("touch", filepath.Join(dir, "README.md")).Run())
	add := exec.Command("git", "add", ".")
	add.Dir = dir
	require.NoError(t, add.Run())
	commit := exec.Command("git", "commit", "-m", "init", "--allow-empty-message")
	commit.Dir = dir
	out, err := commit.CombinedOutput()
	require.NoError(t, err, string(out))
	return dir
}

// addTag creates a lightweight tag in the given repo.
func addTag(t *testing.T, dir, tag string) {
	t.Helper()
	cmd := exec.Command("git", "tag", tag)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// ── ListTags ───────────────────────────────────────────────────────

func TestListTags_NoTags(t *testing.T) {
	repo := initRepo(t, t.TempDir())
	tags, err := ListTags(repo)
	require.NoError(t, err)
	assert.Nil(t, tags)
}

func TestListTags_WithTags(t *testing.T) {
	repo := initRepo(t, t.TempDir())
	addTag(t, repo, "v1.0.0")
	addTag(t, repo, "v2.0.0")

	tags, err := ListTags(repo)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"v1.0.0", "v2.0.0"}, tags)
}

// ── RevParseHEAD ───────────────────────────────────────────────────

func TestRevParseHEAD(t *testing.T) {
	repo := initRepo(t, t.TempDir())
	sha, err := RevParseHEAD(repo)
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{40}$`), sha)
}

func TestRevParseHEAD_NonRepo(t *testing.T) {
	_, err := RevParseHEAD(t.TempDir())
	assert.Error(t, err)
}

// ── Checkout ───────────────────────────────────────────────────────

func TestCheckout_Tag(t *testing.T) {
	repo := initRepo(t, t.TempDir())
	addTag(t, repo, "v1.0.0")

	// add another commit so HEAD moves
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "second")
	cmd.Dir = repo
	require.NoError(t, cmd.Run())

	shaBeforeCheckout, err := RevParseHEAD(repo)
	require.NoError(t, err)

	require.NoError(t, Checkout(repo, "v1.0.0"))

	shaAfterCheckout, err := RevParseHEAD(repo)
	require.NoError(t, err)
	assert.NotEqual(t, shaBeforeCheckout, shaAfterCheckout)
}

func TestCheckout_InvalidRef(t *testing.T) {
	repo := initRepo(t, t.TempDir())
	err := Checkout(repo, "nonexistent-ref-xyz")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git checkout")
}

// ── Clone ──────────────────────────────────────────────────────────

func TestClone(t *testing.T) {
	src := initRepo(t, filepath.Join(t.TempDir(), "src"))
	dest := filepath.Join(t.TempDir(), "dest")

	require.NoError(t, Clone(src, dest))

	sha, err := RevParseHEAD(dest)
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{40}$`), sha)
}

func TestClone_ExistingDest(t *testing.T) {
	src := initRepo(t, filepath.Join(t.TempDir(), "src"))
	dest := t.TempDir() // already exists

	// clone once
	destInner := filepath.Join(dest, "repo")
	require.NoError(t, Clone(src, destInner))

	// clone again to same location should fail
	err := Clone(src, destInner)
	assert.Error(t, err)
}

// ── CloneRef ───────────────────────────────────────────────────────

func TestCloneRef(t *testing.T) {
	src := initRepo(t, filepath.Join(t.TempDir(), "src"))
	addTag(t, src, "v1.0.0")

	// add another commit after the tag
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "post-tag")
	cmd.Dir = src
	require.NoError(t, cmd.Run())

	dest := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, CloneRef(src, dest, "v1.0.0"))

	// HEAD in dest should be at v1.0.0, not the latest commit
	shaTagged, err := RevParseHEAD(dest)
	require.NoError(t, err)

	shaSrc, err := RevParseHEAD(src)
	require.NoError(t, err)
	assert.NotEqual(t, shaSrc, shaTagged)
}

// ── FetchTags ──────────────────────────────────────────────────────

func TestFetchTags_LocalRepo(t *testing.T) {
	src := initRepo(t, filepath.Join(t.TempDir(), "src"))
	addTag(t, src, "v1.0.0")

	dest := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, Clone(src, dest))

	// add a new tag to the source after cloning
	addTag(t, src, "v2.0.0")

	// fetch tags from origin
	require.NoError(t, FetchTags(dest))

	tags, err := ListTags(dest)
	require.NoError(t, err)
	assert.Contains(t, tags, "v2.0.0")
}

// ── ResolveAbsPath ─────────────────────────────────────────────────

func TestResolveAbsPath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "repos", "my-repo")
	expected := filepath.Join(base, "sub", "file.txt")
	assert.Equal(t, expected, ResolveAbsPath(base, "sub/file.txt"))
}

func TestResolveAbsPath_EmptyRel(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "repos", "my-repo")
	assert.Equal(t, base, ResolveAbsPath(base, ""))
}
