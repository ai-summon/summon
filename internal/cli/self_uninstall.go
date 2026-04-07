package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	selfUninstallYes      bool
	selfUninstallKeepData bool
)

var selfUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove summon and all installed data",
	Long:  "Remove the summon binary, clean PATH modifications, and delete the ~/.summon/ data directory.",
	RunE:  runSelfUninstall,
}

func init() {
	selfUninstallCmd.Flags().BoolVar(&selfUninstallYes, "yes", false, "Skip confirmation prompt")
	selfUninstallCmd.Flags().BoolVar(&selfUninstallKeepData, "keep-data", false, "Keep ~/.summon/ data directory")
	selfCmd.AddCommand(selfUninstallCmd)
}

// resolveExpectedBinaryPath returns the expected installer-managed binary path.
// Respects SUMMON_INSTALL_PATH env var; otherwise uses platform-specific defaults.
func resolveExpectedBinaryPath() (string, error) {
	if p := os.Getenv("SUMMON_INSTALL_PATH"); p != "" {
		return expandHome(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, ".summon", "bin", "summon.exe"), nil
	}
	return filepath.Join(home, ".local", "bin", "summon"), nil
}

// resolveDataDir returns the summon data directory path (~/.summon/).
func resolveDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".summon"), nil
}

// detectExternalInstall checks whether the running binary is at the expected
// installer-managed location. Returns a non-nil error with guidance if there's
// a mismatch.
func detectExternalInstall(expectedPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine running executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(expectedPath)
	if err != nil {
		// Expected path may not exist if binary was moved; still compare raw
		resolved = expectedPath
	}

	exePath = filepath.Clean(exePath)
	resolved = filepath.Clean(resolved)

	if !strings.EqualFold(exePath, resolved) {
		msg := fmt.Sprintf(
			"summon appears to have been installed externally (running from %s).\n"+
				"Self-uninstall only supports installations from the official installer (expected at %s).\n\n"+
				"To remove a go-installed binary: rm $(go env GOPATH)/bin/summon\n"+
				"To remove data: rm -rf ~/.summon/",
			exePath, expectedPath,
		)
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// confirmUninstall prompts the user for confirmation. Returns true if the user
// confirms or --yes is set. Returns false if the user declines.
func confirmUninstall(skipConfirm bool) bool {
	if skipConfirm {
		return true
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "Error: no interactive terminal detected. Use --yes to skip confirmation.")
		return false
	}
	return promptConfirm(os.Stdin)
}

// promptConfirm reads a y/N answer from the given reader.
func promptConfirm(r io.Reader) bool {
	fmt.Print("Continue? (y/N): ")
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

// removeDataDir removes the summon data directory. No-op if dir does not exist.
func removeDataDir(dataDir string) error {
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dataDir)
}

// runSelfUninstall is the main orchestration function for self-uninstall.
func runSelfUninstall(cmd *cobra.Command, args []string) error {
	// 1. Resolve footprint
	binaryPath, err := resolveExpectedBinaryPath()
	if err != nil {
		return err
	}
	dataDir, err := resolveDataDir()
	if err != nil {
		return err
	}

	// 2. Detect external installation
	if err := detectExternalInstall(binaryPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// 3. Show removal plan
	fmt.Println("This will remove summon and all installed plugins from your system.")
	fmt.Println()
	fmt.Println("The following will be removed:")
	fmt.Printf("  - Binary: %s\n", binaryPath)
	if !selfUninstallKeepData {
		fmt.Printf("  - Data directory: %s\n", dataDir)
	}
	profileDesc := pathCleanupDescription()
	if profileDesc != "" {
		fmt.Printf("  - %s\n", profileDesc)
	}
	fmt.Println()

	// 4. Confirm
	if !confirmUninstall(selfUninstallYes) {
		fmt.Println("Uninstall cancelled.")
		return nil
	}

	// 5. Execute removals
	hasError := false

	// 5a. Clean PATH
	if err := cleanPath(binaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to clean PATH: %s\n", err)
		hasError = true
	} else {
		fmt.Printf("✓ %s\n", pathCleanupSuccessMessage())
	}

	// 5b. Remove data directory (on Windows this is handled by the GC
	// process because the binary is inside the data dir and locked)
	if selfUninstallKeepData {
		// skip
	} else if dataDirRemovedByGC() {
		// On Windows, the GC process handles data dir removal after the
		// parent exits and the binary lock is released.
	} else {
		if err := removeDataDir(dataDir); err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to remove data directory: %s\n", err)
			hasError = true
		} else {
			fmt.Printf("✓ Removed data directory %s\n", dataDir)
		}
	}

	// 5c. Remove binary (platform-specific)
	if err := removeBinary(binaryPath, dataDir, selfUninstallKeepData); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to remove binary: %s\n", err)
		hasError = true
	} else {
		fmt.Printf("✓ %s\n", binaryRemovalSuccessMessage(binaryPath))
	}

	// 6. Report
	fmt.Println()
	if hasError {
		fmt.Println("Uninstall completed with errors. See above for details.")
		os.Exit(1)
	}
	if selfUninstallKeepData {
		fmt.Printf("summon has been uninstalled. Data directory preserved at %s\n", dataDir)
	} else if dataDirRemovedByGC() {
		fmt.Println("summon has been uninstalled. Data directory will be removed after this process exits.")
	} else {
		fmt.Println("summon has been uninstalled.")
	}
	return nil
}

// expandHome expands ~ to the user's home directory.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
