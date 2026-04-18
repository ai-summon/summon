package resolver

import (
	"fmt"
	"strings"
)

// SourceType categorizes how a dependency specifier should be resolved.
type SourceType int

const (
	SourceOfficialMarketplace SourceType = iota // bare name → official marketplace
	SourceNamedMarketplace                      // name@marketplace
	SourceGitHubShorthand                       // gh:owner/repo
	SourceDirectURL                             // https://... or git://...
	SourceNativeMarketplace                     // native CLI marketplace (no summon-marketplace match)
)

// ResolvedSource represents a resolved dependency source.
type ResolvedSource struct {
	Type           SourceType
	Name           string // package name
	Source         string // resolved source URL or reference
	MarketplaceName string // marketplace name (for SourceNamedMarketplace)
}

// Resolve parses a dependency specifier and returns its resolved source.
func Resolve(specifier string) (*ResolvedSource, error) {
	specifier = strings.TrimSpace(specifier)
	if specifier == "" {
		return nil, fmt.Errorf("empty dependency specifier")
	}

	// gh:owner/repo
	if strings.HasPrefix(specifier, "gh:") {
		parts := strings.TrimPrefix(specifier, "gh:")
		if !strings.Contains(parts, "/") {
			return nil, fmt.Errorf("invalid GitHub shorthand %q: expected gh:owner/repo", specifier)
		}
		return &ResolvedSource{
			Type:   SourceGitHubShorthand,
			Name:   repoNameFromPath(parts),
			Source: fmt.Sprintf("https://github.com/%s", parts),
		}, nil
	}

	// Full URL (https:// or git://)
	if strings.HasPrefix(specifier, "https://") || strings.HasPrefix(specifier, "git://") || strings.HasPrefix(specifier, "http://") {
		return &ResolvedSource{
			Type:   SourceDirectURL,
			Name:   repoNameFromURL(specifier),
			Source: specifier,
		}, nil
	}

	// name@marketplace
	if strings.Contains(specifier, "@") {
		parts := strings.SplitN(specifier, "@", 2)
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid marketplace specifier %q: expected name@marketplace", specifier)
		}
		return &ResolvedSource{
			Type:           SourceNamedMarketplace,
			Name:           parts[0],
			MarketplaceName: parts[1],
		}, nil
	}

	// Bare name → official marketplace
	if !isValidBareName(specifier) {
		return nil, fmt.Errorf("invalid package name %q: must be kebab-case alphanumeric", specifier)
	}
	return &ResolvedSource{
		Type: SourceOfficialMarketplace,
		Name: specifier,
	}, nil
}

func repoNameFromPath(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	return strings.TrimSuffix(name, ".git")
}

func repoNameFromURL(url string) string {
	// Strip scheme
	path := url
	for _, prefix := range []string{"https://", "http://", "git://"} {
		path = strings.TrimPrefix(path, prefix)
	}
	// Get last path segment
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	name := parts[len(parts)-1]
	return strings.TrimSuffix(name, ".git")
}

func isValidBareName(name string) bool {
	if len(name) == 0 || len(name) > 50 {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return name[0] != '-' && name[len(name)-1] != '-'
}
