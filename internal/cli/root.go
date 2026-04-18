package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	targetFlag  string
	noColorFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "summon",
	Short: "The dependency manager for AI plugins",
	Long:  `The dependency manager for AI plugins.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&targetFlag, "target", "", "Target a specific CLI: copilot or claude")
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false, "Disable colored output")
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	// Command groups for organized help output
	rootCmd.AddGroup(
		&cobra.Group{ID: "packages", Title: "Plugin Management:"},
		&cobra.Group{ID: "inspect", Title: "Inspection:"},
		&cobra.Group{ID: "config", Title: "Configuration:"},
		&cobra.Group{ID: "maintain", Title: "Maintenance:"},
	)

	// Custom styled help and usage rendering
	configureHelp(rootCmd)
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
