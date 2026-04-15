package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	targetFlag string
)

var rootCmd = &cobra.Command{
	Use:   "summon",
	Short: "Unified plugin dependency manager for AI CLIs",
	Long: `Summon is a unified plugin dependency manager for AI CLIs (Copilot CLI and Claude Code CLI).
It resolves transitive dependencies, checks system prerequisites, and provides a unified
install/uninstall experience while delegating actual plugin operations to the native CLIs.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&targetFlag, "target", "", "Target a specific CLI: copilot or claude")
}

// SetVersion configures the version string displayed by --version.
func SetVersion(v string) {
	rootCmd.Version = v
	rootCmd.SetVersionTemplate(fmt.Sprintf("summon version %s\n", v))
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// GetRootCmd returns the root command for testing.
func GetRootCmd() *cobra.Command {
	return rootCmd
}
