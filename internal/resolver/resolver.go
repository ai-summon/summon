// Package resolver picks the best git ref for a dependency.
// It inspects local (or remote) tags, filters for valid semver strings,
// and returns the highest version. When no semver tags are found it
// falls back to "HEAD".
package resolver

import (
	"sort"
	"strconv"
	"strings"

	"github.com/user/summon/internal/git"
)

// semverParts parses a version string like "v1.2.3" or "1.2.3" into
// (major, minor, patch). Pre-release suffixes (e.g. "-beta.1") are
// stripped before parsing. Returns ok=false if the string is not a
// valid three-part version.
func semverParts(v string) (int, int, int, bool) {
	v = strings.TrimPrefix(v, "v")
	// Strip any pre-release suffix for sorting
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

// isSemver checks if a tag looks like a semver tag.
func isSemver(tag string) bool {
	_, _, _, ok := semverParts(tag)
	return ok
}

// sortSemverTags sorts semver tags in ascending order.
func sortSemverTags(tags []string) {
	sort.Slice(tags, func(i, j int) bool {
		mi, mni, pi, _ := semverParts(tags[i])
		mj, mnj, pj, _ := semverParts(tags[j])
		if mi != mj {
			return mi < mj
		}
		if mni != mnj {
			return mni < mnj
		}
		return pi < pj
	})
}

// ResolveLatest finds the latest semver tag in the local repository at
// repoDir. If no semver tags exist it returns "HEAD".
func ResolveLatest(repoDir string) (string, error) {
	tags, err := git.ListTags(repoDir)
	if err != nil {
		return "HEAD", nil
	}
	var semverTags []string
	for _, t := range tags {
		if isSemver(t) {
			semverTags = append(semverTags, t)
		}
	}
	if len(semverTags) == 0 {
		return "HEAD", nil
	}
	sortSemverTags(semverTags)
	return semverTags[len(semverTags)-1], nil
}

// ResolveLatestRemote queries the remote at url for tags and returns
// the highest semver tag without cloning the repository.
func ResolveLatestRemote(url string) (string, error) {
	tags, err := git.LsRemote(url)
	if err != nil {
		return "HEAD", nil
	}
	var semverTags []string
	for _, t := range tags {
		if isSemver(t) {
			semverTags = append(semverTags, t)
		}
	}
	if len(semverTags) == 0 {
		return "HEAD", nil
	}
	sortSemverTags(semverTags)
	return semverTags[len(semverTags)-1], nil
}
