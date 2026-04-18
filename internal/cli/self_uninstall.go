package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ai-summon/summon/internal/selfmgmt"
	"github.com/spf13/cobra"
)

var selfUninstallConfirm bool

// selfUninstallDeps holds injectable dependencies for the self uninstall command.
type selfUninstallDeps struct {
	pathResolver selfmgmt.PathResolver
	fileSystem   selfmgmt.FileSystem
	noColor      bool
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
}

func defaultSelfUninstallDeps() *selfUninstallDeps {
	return &selfUninstallDeps{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

var selfUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove summon from your system",
	Long:  `Remove the summon binary and configuration directory from your system.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := defaultSelfUninstallDeps()
		deps.noColor = noColorFlag
		return runSelfUninstall(deps)
	},
}

func init() {
	selfUninstallCmd.Flags().BoolVar(&selfUninstallConfirm, "confirm", false, "Skip confirmation prompt (for CI/automation)")
	selfCmd.AddCommand(selfUninstallCmd)
}

func runSelfUninstall(deps *selfUninstallDeps) error {
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
		return fmt.Errorf("error: %w", err)
	}

	// Display paths to be removed
	_, _ = fmt.Fprintln(out, "This will remove:")

	// Check if config dir exists to decide whether to display it
	var configExists bool
	if deps.fileSystem != nil {
		_, statErr := deps.fileSystem.Stat(paths.ConfigDir)
		configExists = statErr == nil
	} else {
		_, statErr := os.Stat(paths.ConfigDir)
		configExists = statErr == nil
	}

	if configExists {
		_, _ = fmt.Fprintf(out, "    %s\n", paths.ConfigDir)
	}
	_, _ = fmt.Fprintf(out, "    %s\n", paths.BinaryPath)
	_, _ = fmt.Fprintln(out)

	// Confirmation prompt (unless --confirm)
	if !selfUninstallConfirm {
		_, _ = fmt.Fprint(out, "Remove summon and all configuration data? [y/N] ")

		scanner := bufio.NewScanner(deps.stdin)
		if scanner.Scan() {
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "y" && answer != "yes" {
				return nil // User declined — exit cleanly
			}
		} else {
			return nil // No input — exit cleanly
		}
	}

	// Perform uninstall
	if deps.fileSystem != nil {
		err = selfmgmt.UninstallWith(paths, out, deps.fileSystem)
	} else {
		err = selfmgmt.Uninstall(paths, out)
	}
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "summon is now uninstalled")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Note: plugins installed in native CLI platforms (copilot, claude) are not")
	_, _ = fmt.Fprintln(out, "removed. Use each platform's tools to manage those plugins.")

	return nil
}
