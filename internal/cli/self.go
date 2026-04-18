package cli

import "github.com/spf13/cobra"

var selfCmd = &cobra.Command{
	Use:     "self <command>",
	Short:   "Manage the summon installation",
	GroupID: "maintain",
	Long:    `Manage the summon installation, including self-update and uninstall.`,
}

func init() {
	rootCmd.AddCommand(selfCmd)
}
