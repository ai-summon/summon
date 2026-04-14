package git

import (
	"fmt"
	"strings"
)

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// Client provides git operations.
type Client struct {
	runner CommandRunner
}

// NewClient creates a new git Client.
func NewClient(runner CommandRunner) *Client {
	return &Client{runner: runner}
}

// RepoExists checks if a remote repository exists using git ls-remote.
func (c *Client) RepoExists(url string) (bool, error) {
	_, err := c.runner.Run("git", "ls-remote", "--exit-code", url)
	if err != nil {
		if strings.Contains(err.Error(), "exit status") {
			return false, nil
		}
		return false, fmt.Errorf("git ls-remote failed: %w", err)
	}
	return true, nil
}

// ArchiveFile extracts a single file from a remote repo using git archive.
func (c *Client) ArchiveFile(url, ref, filename string) ([]byte, error) {
	output, err := c.runner.Run("git", "archive", "--remote="+url, ref, filename)
	if err != nil {
		return nil, fmt.Errorf("git archive failed for %s: %w", filename, err)
	}
	return output, nil
}

// ShallowClone performs a shallow clone of a repository.
func (c *Client) ShallowClone(url, dest string) error {
	_, err := c.runner.Run("git", "clone", "--depth", "1", "--filter=blob:none", url, dest)
	if err != nil {
		return fmt.Errorf("git shallow clone failed: %w", err)
	}
	return nil
}
