// Package git provides a thin wrapper around the git CLI.
// Each function shells out to the git binary, so a working git
// installation is required at runtime.
package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clone clones the repository at url into dest.
// Returns an error if dest already exists or the clone fails.
func Clone(url, dest string) error {
	cmd := exec.Command("git", "clone", "--quiet", url, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Fetch fetches all refs from the configured remote into the local
// repository at repoDir.
// FetchTags fetches all tags from the configured remote into the
// local repository at repoDir.
func FetchTags(repoDir string) error {
	cmd := exec.Command("git", "fetch", "--tags", "--quiet")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListTags returns all tags present in the local repository at repoDir.
// Returns nil (not an empty slice) when no tags exist.
func ListTags(repoDir string) ([]string, error) {
	cmd := exec.Command("git", "tag", "--list")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git tag --list: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// Checkout switches the working tree in repoDir to the given ref,
// which may be a tag name, branch name, or commit SHA.
func Checkout(repoDir, ref string) error {
	cmd := exec.Command("git", "checkout", "--quiet", ref)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RevParseHEAD returns the full 40-character hex SHA of the current
// HEAD commit in the repository at repoDir.
func RevParseHEAD(repoDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// LsRemote queries the remote at url for tag refs without cloning.
// It returns tag names with the "refs/tags/" prefix stripped.
func LsRemote(url string) ([]string, error) {
	cmd := exec.Command("git", "ls-remote", "--tags", "--refs", url)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-remote: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	var tags []string
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			ref := parts[1]
			tag := strings.TrimPrefix(ref, "refs/tags/")
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// CloneRef is a convenience that clones the repository at url into dest
// and then checks out the specified ref (tag, branch, or SHA).
func CloneRef(url, dest, ref string) error {
	if err := Clone(url, dest); err != nil {
		return err
	}
	return Checkout(dest, ref)
}

// Pull fetches from the remote and fast-forwards the current branch
// in the repository at repoDir.
func Pull(repoDir string) error {
	cmd := exec.Command("git", "pull", "--quiet")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ResolveAbsPath joins repoDir and rel into an absolute file path.
// It is a pure function and does not touch the filesystem.
func ResolveAbsPath(repoDir, rel string) string {
	return filepath.Join(repoDir, rel)
}
