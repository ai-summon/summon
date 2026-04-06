package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCmd_Metadata(t *testing.T) {
	assert.Equal(t, "summon", rootCmd.Use)
	assert.Equal(t, version, rootCmd.Version)
	assert.NotEmpty(t, rootCmd.Long)
}

func TestRootCmd_SubcommandsRegistered(t *testing.T) {
	names := make(map[string]bool)
	for _, sub := range rootCmd.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"init", "install", "list", "uninstall", "update"} {
		assert.True(t, names[want], "subcommand %q should be registered", want)
	}
}
