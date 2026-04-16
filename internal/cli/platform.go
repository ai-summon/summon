package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ai-summon/summon/internal/config"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// platformDeps holds injectable dependencies for the platform command.
type platformDeps struct {
	runner     platform.CommandRunner
	stdout     io.Writer
	stderr     io.Writer
	configPath string // override for testing; empty = default
	noColor    bool
}

func defaultPlatformDeps() *platformDeps {
	return &platformDeps{
		runner: &execRunner{},
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

var platformCmd = &cobra.Command{
	Use:   "platform",
	Short: "Manage platform connections",
	Long:  `View and manage which AI CLI platforms (copilot, claude) summon operates on.`,
}

var platformListCmd = &cobra.Command{
	Use:   "list",
	Short: "List platforms and their status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlatformList(defaultPlatformDeps())
	},
}

var platformEnableCmd = &cobra.Command{
	Use:   "enable <platform>",
	Short: "Enable a platform",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlatformToggle(args[0], true, defaultPlatformDeps())
	},
}

var platformDisableCmd = &cobra.Command{
	Use:   "disable <platform>",
	Short: "Disable a platform",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlatformToggle(args[0], false, defaultPlatformDeps())
	},
}

func init() {
	platformCmd.AddCommand(platformListCmd)
	platformCmd.AddCommand(platformEnableCmd)
	platformCmd.AddCommand(platformDisableCmd)
	rootCmd.AddCommand(platformCmd)
}

func runPlatformList(deps *platformDeps) error {
	cfgPath, cfg, err := loadPlatformConfig(deps)
	if err != nil {
		return err
	}
	_ = cfgPath

	// Detect what's actually available
	detected := platform.DetectAdapters(deps.runner)
	detectedSet := make(map[string]bool, len(detected))
	for _, a := range detected {
		detectedSet[a.Name()] = true
	}

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	crossStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dimStyle := lipgloss.NewStyle().Faint(true)
	if deps.noColor {
		headerStyle = lipgloss.NewStyle()
		checkStyle = lipgloss.NewStyle()
		crossStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	fmt.Fprintln(deps.stdout, headerStyle.Render("Platforms:"))
	fmt.Fprintln(deps.stdout)

	for _, name := range config.KnownPlatforms() {
		enabled, configured := cfg.IsEnabled(name)
		available := detectedSet[name]

		// A platform is active if explicitly enabled OR auto-detected without config
		active := (configured && enabled) || (!configured && available)

		var statusIcon, statusText, detail string

		if active && available {
			statusIcon = checkStyle.Render("✓")
			statusText = "enabled"
			detail = ""
		} else if active && !available {
			// Enabled in config but CLI not found
			statusIcon = crossStyle.Render("!")
			statusText = "enabled"
			detail = dimStyle.Render("(not installed)")
		} else if configured && !enabled {
			statusIcon = dimStyle.Render("–")
			statusText = "disabled"
			detail = ""
		} else {
			// Not configured and not available
			statusIcon = crossStyle.Render("✗")
			statusText = "not installed"
			detail = ""
		}

		line := fmt.Sprintf("  %s %-10s %s", statusIcon, name, statusText)
		if detail != "" {
			line += " " + detail
		}
		fmt.Fprintln(deps.stdout, line)
	}

	return nil
}

func runPlatformToggle(name string, enable bool, deps *platformDeps) error {
	// Validate platform name
	known := false
	for _, k := range config.KnownPlatforms() {
		if k == name {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("unknown platform %q; known platforms: %v", name, config.KnownPlatforms())
	}

	cfgPath, cfg, err := loadPlatformConfig(deps)
	if err != nil {
		return err
	}

	if err := cfg.SetPlatform(name, enable); err != nil {
		return err
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	action := "enabled"
	if !enable {
		action = "disabled"
	}

	// Warn if enabling a platform that's not installed
	if enable {
		detected := platform.DetectAdapters(deps.runner)
		found := false
		for _, a := range detected {
			if a.Name() == name {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(deps.stderr, "⚠ %s CLI is not installed; it will be used once available\n", name)
		}
	}

	fmt.Fprintf(deps.stdout, "%s %s\n", name, action)
	return nil
}

func loadPlatformConfig(deps *platformDeps) (string, config.Config, error) {
	cfgPath := deps.configPath
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultPath()
		if err != nil {
			return "", config.Config{}, fmt.Errorf("cannot determine config path: %w", err)
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", config.Config{}, fmt.Errorf("loading config: %w", err)
	}
	return cfgPath, cfg, nil
}
