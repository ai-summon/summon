package depcheck

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// ParseConstraint parses a version constraint string.
// An empty string means "any version" (returns nil constraint, no error).
// Returns an error for unparseable constraints.
func ParseConstraint(s string) (*semver.Constraints, error) {
	if s == "" {
		return nil, nil
	}
	c, err := semver.NewConstraint(s)
	if err != nil {
		return nil, fmt.Errorf("invalid version constraint %q: %w", s, err)
	}
	return c, nil
}

// CheckVersion checks if an installed version satisfies a constraint string.
// An empty constraint means "any version" — always satisfied.
// Returns (satisfied, error). Error is non-nil for unparseable versions or constraints.
func CheckVersion(installedVersion, constraint string) (bool, error) {
	if constraint == "" {
		return true, nil
	}
	c, err := ParseConstraint(constraint)
	if err != nil {
		return false, err
	}
	v, err := semver.NewVersion(installedVersion)
	if err != nil {
		return false, fmt.Errorf("invalid version %q: %w", installedVersion, err)
	}
	return c.Check(v), nil
}
