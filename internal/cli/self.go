package cli

import "github.com/spf13/cobra"

var selfCmd = &cobra.Command{
	Use:   "self",
	Short: "Manage the summon installation itself",
}

func init() {
	rootCmd.AddCommand(selfCmd)
}
