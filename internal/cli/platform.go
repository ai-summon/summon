package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ai-summon/summon/internal/config"
	"github.com/ai-summon/summon/internal/platform"
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
	Use:     "platform",
	Short:   "Manage platform connections",
	GroupID: "config",
	Long:    `View and manage which AI CLI platforms (copilot, claude) summon operates on.`,
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
	s := NewStyles(deps.noColor)

	_, _ = fmt.Fprintln(deps.stdout, s.Header.Render("Platforms:"))
	_, _ = fmt.Fprintln(deps.stdout)

	for _, name := range config.KnownPlatforms() {
		enabled, configured := cfg.IsEnabled(name)
		available := detectedSet[name]

		var statusIcon, statusText, detail string

		switch {
		case configured && enabled && available:
			statusIcon = s.Success.Render("✓")
			statusText = "enabled"
			detail = ""
		case configured && enabled && !available:
			statusIcon = s.Error.Render("!")
			statusText = "enabled"
			detail = s.Dim.Render("(not installed)")
		case configured && !enabled:
			statusIcon = s.Dim.Render("–")
			statusText = "disabled"
			detail = ""
		case !configured && available:
			statusIcon = s.Dim.Render("○")
			statusText = "detected"
			detail = s.Dim.Render("(not enabled)")
		default:
			statusIcon = s.Error.Render("✗")
			statusText = "not installed"
			detail = ""
		}

		line := fmt.Sprintf("  %s %-10s %s", statusIcon, name, statusText)
		if detail != "" {
			line += " " + detail
		}
		_, _ = fmt.Fprintln(deps.stdout, line)
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
			_, _ = fmt.Fprintf(deps.stderr, "⚠ %s CLI is not installed; it will be used once available\n", name)
		}
	}

	_, _ = fmt.Fprintf(deps.stdout, "%s %s\n", name, action)
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
