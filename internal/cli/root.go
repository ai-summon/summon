// Package cli wires up the cobra command tree for the summon binary.
// Each subcommand (init, install, list, new, self, uninstall, update) is
// defined in its own file and registered with the root command via init()
// functions.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// version, commit, and date are stamped at build time via ldflags;
// defaults are used for local dev builds.
var (
	version = "0.1.0"
	commit  = "none"
	date    = "unknown"
)

// rootCmd is the top-level cobra command. All subcommands attach to it.
var rootCmd = &cobra.Command{
	Use:           "summon",
	Short:         "AI agent package manager",
	Long:          "Summon is a cross-platform package manager for AI agent components (skills, agents, commands, hooks, MCP servers).",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	if commit != "none" {
		// Truncate date to YYYY-MM-DD if it contains a 'T' (RFC 3339).
		if i := strings.IndexByte(date, 'T'); i > 0 {
			date = date[:i]
		}
		rootCmd.Version = version + " (" + commit + " " + date + ")"
	}
}

// Execute runs the root command. It prints errors to stderr and exits
// with code 1 on failure. This is the single entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
