package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/ai-summon/summon/internal/selfmgmt"
	"github.com/spf13/cobra"
)

// selfUpdateDeps holds injectable dependencies for the self update command.
type selfUpdateDeps struct {
	httpClient   selfmgmt.HTTPClient
	execRunner   selfmgmt.ExecRunner
	pathResolver selfmgmt.PathResolver
	stdout       io.Writer
	stderr       io.Writer
}

func defaultSelfUpdateDeps() *selfUpdateDeps {
	return &selfUpdateDeps{
		httpClient: http.DefaultClient,
		execRunner: &selfmgmt.ExecRunnerAdapter{},
		stdout:     os.Stdout,
		stderr:     os.Stderr,
	}
}

var selfUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update summon to the latest version",
	Long:  `Check for a newer version of summon and update the binary in-place.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultSelfUpdateDeps()
		return runSelfUpdate(deps)
	},
}

func init() {
	selfCmd.AddCommand(selfUpdateCmd)
}

func runSelfUpdate(deps *selfUpdateDeps) error {
	out := deps.stdout

	// Resolve paths
	var paths selfmgmt.SummonPaths
	var err error
	if deps.pathResolver != nil {
		paths, err = selfmgmt.ResolvePathsWith(deps.pathResolver)
	} else {
		paths, err = selfmgmt.ResolvePaths()
	}
	if err != nil {
		return fmt.Errorf("Error: %w", err)
	}

	// Get compiled-in version
	currentVersion := rootCmd.Version
	if currentVersion == "" {
		currentVersion = "dev"
	}

	// Run update
	result, err := selfmgmt.RunUpdate(currentVersion, paths, deps.httpClient, deps.execRunner, out)
	if err != nil {
		return err
	}

	if result.AlreadyUpToDate {
		fmt.Fprintf(out, "summon v%s is already up to date\n", result.CurrentVersion)
		return nil
	}

	if result.Updated {
		fmt.Fprintln(out, "updated successfully")
	}

	return nil
}
