// Package cli wires up the cobra command tree for the summon binary.
// Each subcommand (init, install, list, uninstall, update) is defined in its
// own file and registered with the root command via init() functions.
package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/user/summon/internal/ui"
)

// version is stamped at build time via ldflags; defaults to dev value.
var version = "0.1.0"

// rootCmd is the top-level cobra command. All subcommands attach to it.
var rootCmd = &cobra.Command{
	Use:           "summon",
	Short:         "AI agent package manager",
	Long:          "Summon is a cross-platform package manager for AI agent components (skills, agents, commands, hooks, MCP servers).",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
			ui.SetColorEnabled(false)
		}
	},
}

var noColorFlag bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false, "Disable colored output")
}

// Execute runs the root command. It prints errors to stderr and exits
// with code 1 on failure. This is the single entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		ui.Error("%s", err)
		os.Exit(1)
	}
}
