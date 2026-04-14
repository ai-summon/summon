package git

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGitRunner struct {
	commands [][]string
	runFunc  func(name string, args ...string) ([]byte, error)
}

func (f *fakeGitRunner) Run(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, append([]string{name}, args...))
	if f.runFunc != nil {
		return f.runFunc(name, args...)
	}
	return nil, nil
}

func TestRepoExists_Found(t *testing.T) {
	runner := &fakeGitRunner{}
	client := NewClient(runner)
	exists, err := client.RepoExists("https://github.com/owner/repo")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []string{"git", "ls-remote", "--exit-code", "https://github.com/owner/repo"}, runner.commands[0])
}

func TestRepoExists_NotFound(t *testing.T) {
	runner := &fakeGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exit status 2")
		},
	}
	client := NewClient(runner)
	exists, err := client.RepoExists("https://github.com/owner/nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestArchiveFile_Success(t *testing.T) {
	runner := &fakeGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("file content"), nil
		},
	}
	client := NewClient(runner)
	data, err := client.ArchiveFile("https://github.com/owner/repo", "HEAD", "summon.yaml")
	require.NoError(t, err)
	assert.Equal(t, "file content", string(data))
}

func TestArchiveFile_Failure(t *testing.T) {
	runner := &fakeGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("archive not supported")
		},
	}
	client := NewClient(runner)
	_, err := client.ArchiveFile("https://github.com/owner/repo", "HEAD", "summon.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git archive failed")
}

func TestShallowClone_Success(t *testing.T) {
	runner := &fakeGitRunner{}
	client := NewClient(runner)
	err := client.ShallowClone("https://github.com/owner/repo", "/tmp/clone")
	require.NoError(t, err)
	assert.Contains(t, runner.commands[0], "--depth")
	assert.Contains(t, runner.commands[0], "1")
}

func TestShallowClone_Failure(t *testing.T) {
	runner := &fakeGitRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("clone failed")
		},
	}
	client := NewClient(runner)
	err := client.ShallowClone("https://github.com/owner/repo", "/tmp/clone")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shallow clone failed")
}
