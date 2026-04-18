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
	// noColor disables colored output (used in tests).
	noColor bool
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
	s := NewStyles(deps.noColor)

	infoPrefix := s.Header.Render("info:")
	successPrefix := s.Success.Render("success:")

	// Get compiled-in version
	currentVersion := rootCmd.Version
	if currentVersion == "" {
		currentVersion = "dev"
	}
	current := selfmgmt.StripVersion(currentVersion)

	// Check latest version
	_, _ = fmt.Fprintf(out, "%s Checking for updates...\n", infoPrefix)

	release, err := selfmgmt.FetchLatestVersion(deps.httpClient)
	if err != nil {
		return err
	}

	if selfmgmt.IsUpToDate(current, release.Version) {
		_, _ = fmt.Fprintf(out, "%s You're already on version v%s of summon (the latest version).\n", successPrefix, current)
		return nil
	}

	_, _ = fmt.Fprintf(out, "%s Updating summon v%s → v%s\n", infoPrefix, current, release.Version)

	// Resolve paths only when an update is needed
	var paths selfmgmt.SummonPaths
	if deps.pathResolver != nil {
		paths, err = selfmgmt.ResolvePathsWith(deps.pathResolver)
	} else {
		paths, err = selfmgmt.ResolvePaths()
	}
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	if err := selfmgmt.PerformUpdate(release, paths, deps.httpClient, deps.execRunner, out); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "%s Updated summon to v%s!\n", successPrefix, release.Version)
	return nil
}
