package cli

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ai-summon/summon/internal/selfmgmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// versionCheckDeps holds injectable dependencies for version checking.
type versionCheckDeps struct {
	httpClient selfmgmt.HTTPClient
	configDir  string
	isTTY      func() bool
}

func defaultVersionCheckDeps() *versionCheckDeps {
	return &versionCheckDeps{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		isTTY: func() bool {
			return term.IsTerminal(int(os.Stderr.Fd()))
		},
	}
}

// versionCheckHook is the PersistentPreRun function that checks for updates.
// It is best-effort: errors are silently ignored.
var versionCheckHook = func(cmd *cobra.Command, args []string) {
	runVersionCheck(cmd, defaultVersionCheckDeps())
}

func runVersionCheck(cmd *cobra.Command, deps *versionCheckDeps) {
	// Skip if disabled via env var.
	if os.Getenv("SUMMON_NO_UPDATE_CHECK") == "1" {
		return
	}

	currentVersion := rootCmd.Version
	if currentVersion == "" || currentVersion == "dev" {
		return
	}

	// Skip for commands where a version warning is redundant or unwanted.
	if shouldSkipVersionCheck(cmd) {
		return
	}

	// Only warn when stderr is a terminal.
	if !deps.isTTY() {
		return
	}

	// Resolve config directory.
	configDir := deps.configDir
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		configDir = home + "/.summon"
	}

	result := selfmgmt.CheckVersionCache(configDir, currentVersion)

	if result.UpdateAvailable {
		boxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("3")).
			Padding(1, 3)
		content := fmt.Sprintf("Update available: v%s → v%s\nRun \"summon self update\" to upgrade", result.CurrentVersion, result.LatestVersion)
		fmt.Fprintf(os.Stderr, "\n%s\n\n", boxStyle.Render(content))
	}

	if result.NeedsRefresh {
		go selfmgmt.RefreshVersionCache(configDir, deps.httpClient)
	}
}

// shouldSkipVersionCheck returns true for commands where version warnings
// would be redundant or inappropriate.
func shouldSkipVersionCheck(cmd *cobra.Command) bool {
	name := cmd.Name()

	// "summon self update" already handles version checking.
	if name == "update" && cmd.Parent() != nil && cmd.Parent().Name() == "self" {
		return true
	}

	// "summon --version" is a version display command.
	if name == "summon" || name == "" {
		// Check if --version flag is being used (cobra handles this internally,
		// but the PersistentPreRun fires before the version flag handler).
		if cmd.Flags().Changed("version") {
			return true
		}
	}

	// Skip for help and completion commands.
	if name == "help" || name == "completion" {
		return true
	}

	return false
}

func init() {
	rootCmd.PersistentPreRun = versionCheckHook
}
