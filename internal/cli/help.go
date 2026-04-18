package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// rpad pads s on the right to width w using spaces.
func rpad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// configureHelp sets custom help and usage rendering on the root command.
// Colors cascade to all subcommands via Cobra's parent lookup.
func configureHelp(root *cobra.Command) {
	root.SetHelpFunc(helpFunc)
	root.SetUsageFunc(usageFunc)
	root.InitDefaultHelpCmd()
	for _, sub := range root.Commands() {
		if sub.Name() == "help" {
			sub.GroupID = "maintain"
			break
		}
	}
}

func helpFunc(cmd *cobra.Command, _ []string) {
	s := NewStyles(noColorFlag)
	w := cmd.OutOrStdout()

	desc := cmd.Long
	if desc == "" {
		desc = cmd.Short
	}
	desc = strings.TrimRight(desc, " \t\n")
	if desc != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, desc)
		_, _ = fmt.Fprintln(w)
	}

	if cmd.Runnable() || cmd.HasSubCommands() {
		_ = usageFuncWriter(w, cmd, s)
	}
}

func usageFunc(cmd *cobra.Command) error {
	s := NewStyles(noColorFlag)
	return usageFuncWriter(cmd.OutOrStdout(), cmd, s)
}

// usageFuncWriter renders styled usage to w.
// Mirrors cobra's defaultUsageFunc but adds color via Styles.
func usageFuncWriter(w io.Writer, c *cobra.Command, s Styles) error {
	heading := func(text string) string { return s.HelpHeading.Render(text) }
	cmdStyle := func(text string) string { return s.HelpCommand.Render(text) }

	// Usage
	_, _ = fmt.Fprint(w, heading("Usage:"))
	if c.Runnable() {
		_, _ = fmt.Fprintf(w, "\n  %s", c.UseLine())
	}
	if c.HasAvailableSubCommands() {
		_, _ = fmt.Fprintf(w, "\n  %s [command]", c.CommandPath())
	}

	// Aliases
	if len(c.Aliases) > 0 {
		_, _ = fmt.Fprintf(w, "\n\n%s\n", heading("Aliases:"))
		_, _ = fmt.Fprintf(w, "  %s", c.NameAndAliases())
	}

	// Examples
	if c.HasExample() {
		_, _ = fmt.Fprintf(w, "\n\n%s\n", heading("Examples:"))
		_, _ = fmt.Fprint(w, c.Example)
	}

	// Commands (grouped or flat)
	if c.HasAvailableSubCommands() {
		cmds := c.Commands()
		if len(c.Groups()) == 0 {
			_, _ = fmt.Fprintf(w, "\n\n%s", heading("Available Commands:"))
			for _, sub := range cmds {
				if sub.IsAvailableCommand() || sub.Name() == "help" {
					_, _ = fmt.Fprintf(w, "\n  %s %s",
						cmdStyle(rpad(sub.Name(), sub.NamePadding())),
						sub.Short)
				}
			}
		} else {
			for _, group := range c.Groups() {
				_, _ = fmt.Fprintf(w, "\n\n%s", heading(group.Title))
				for _, sub := range cmds {
					if sub.GroupID == group.ID && (sub.IsAvailableCommand() || sub.Name() == "help") {
						_, _ = fmt.Fprintf(w, "\n  %s %s",
							cmdStyle(rpad(sub.Name(), sub.NamePadding())),
							sub.Short)
					}
				}
			}
			if !c.AllChildCommandsHaveGroup() {
				_, _ = fmt.Fprintf(w, "\n\n%s", heading("Additional Commands:"))
				for _, sub := range cmds {
					if sub.GroupID == "" && (sub.IsAvailableCommand() || sub.Name() == "help") {
						_, _ = fmt.Fprintf(w, "\n  %s %s",
							cmdStyle(rpad(sub.Name(), sub.NamePadding())),
							sub.Short)
					}
				}
			}
		}
	}

	// Flags
	if c.HasAvailableLocalFlags() {
		_, _ = fmt.Fprintf(w, "\n\n%s\n", heading("Flags:"))
		_, _ = fmt.Fprint(w, styleFlagUsages(c.LocalFlags().FlagUsages(), cmdStyle))
	}
	if c.HasAvailableInheritedFlags() {
		_, _ = fmt.Fprintf(w, "\n\n%s\n", heading("Global Flags:"))
		_, _ = fmt.Fprint(w, styleFlagUsages(c.InheritedFlags().FlagUsages(), cmdStyle))
	}

	// Additional help topics
	if c.HasHelpSubCommands() {
		_, _ = fmt.Fprintf(w, "\n\n%s", heading("Additional help topics:"))
		for _, sub := range c.Commands() {
			if sub.IsAdditionalHelpTopicCommand() {
				_, _ = fmt.Fprintf(w, "\n  %s %s",
					cmdStyle(rpad(sub.CommandPath(), sub.CommandPathPadding())),
					sub.Short)
			}
		}
	}

	// Footer
	if c.HasAvailableSubCommands() {
		_, _ = fmt.Fprintf(w, "\n\nUse \"%s [command] --help\" for more information about a command.", c.CommandPath())
	}
	_, _ = fmt.Fprintln(w)
	return nil
}

// styleFlagUsages colorizes flag names in cobra's FlagUsages() output.
// Each line has the form "      --flag-name type   Description".
// We colorize the flag portion (everything before the description).
func styleFlagUsages(usages string, cmdStyle func(string) string) string {
	usages = strings.TrimRight(usages, " \t\n")
	lines := strings.Split(usages, "\n")
	var out strings.Builder
	for i, line := range lines {
		styled := styleFlagLine(line, cmdStyle)
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(styled)
	}
	return out.String()
}

// styleFlagLine colorizes the flag portion of a single flag-usage line.
// Cobra formats flags as: "  -s, --long type    description"
// We find where the description starts and colorize the flag tokens before it.
func styleFlagLine(line string, cmdStyle func(string) string) string {
	if len(strings.TrimSpace(line)) == 0 {
		return line
	}

	// Find the flag part vs description part.
	// Cobra uses at least 2 consecutive spaces to separate flags from description.
	// We look for the pattern: non-space, then 2+ spaces, then non-space.
	flagEnd := findDescriptionStart(line)
	if flagEnd < 0 {
		// Entire line is the flag (no description) — colorize it all
		leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
		return strings.Repeat(" ", leadingSpaces) + cmdStyle(strings.TrimLeft(line, " "))
	}

	flagPart := line[:flagEnd]
	descPart := line[flagEnd:]

	// Preserve leading whitespace, colorize the flag tokens
	leadingSpaces := len(flagPart) - len(strings.TrimLeft(flagPart, " "))
	flagTokens := strings.TrimLeft(flagPart, " ")

	return strings.Repeat(" ", leadingSpaces) + cmdStyle(flagTokens) + descPart
}

// findDescriptionStart returns the index where the description text starts,
// or -1 if there is no description (just flag text).
// Cobra separates flag from description with 3+ spaces after the flag/type.
func findDescriptionStart(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	if len(trimmed) == 0 {
		return -1
	}
	offset := len(line) - len(trimmed)

	inSpaces := false
	spaceStart := 0
	for i, ch := range trimmed {
		if ch == ' ' {
			if !inSpaces {
				inSpaces = true
				spaceStart = i
			}
		} else {
			if inSpaces && (i-spaceStart) >= 3 {
				return offset + i
			}
			inSpaces = false
		}
	}
	return -1
}
