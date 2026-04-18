package cli

import "github.com/charmbracelet/lipgloss"

// Styles holds the shared style definitions for all CLI output rendering.
// Constructed per-invocation via NewStyles; not persisted.
type Styles struct {
	Header  lipgloss.Style // Bold cyan — section headers
	Success lipgloss.Style // Green — checkmarks, success indicators
	Error   lipgloss.Style // Red — errors, failures
	Warn    lipgloss.Style // Yellow — warnings
	Dim     lipgloss.Style // Faint — secondary info, metadata
	Name    lipgloss.Style // Green — package/plugin names
	URL     lipgloss.Style // Faint — source URLs
	Star    lipgloss.Style // Yellow — official marketplace badge (★)
	Bullet  lipgloss.Style // Green — non-official marketplace badge (●)
	Box     lipgloss.Style // Rounded border — notification boxes

	// Help-specific styles (used by custom help/usage templates)
	HelpHeading lipgloss.Style // Bold green — help section headers ("Usage:", "Commands:")
	HelpCommand lipgloss.Style // Bold cyan — command/flag names in help output
}

// NewStyles creates the style set for CLI output rendering.
// When noColor is true, returns styles with no ANSI formatting.
func NewStyles(noColor bool) Styles {
	if noColor {
		return Styles{
			// Box keeps structural layout (border + padding) but no color
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Padding(1, 3),
		}
	}
	return Styles{
		Header:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		Warn:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Dim:     lipgloss.NewStyle().Faint(true),
		Name:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		URL:     lipgloss.NewStyle().Faint(true),
		Star:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Bullet:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("3")).
			Padding(1, 3),
		HelpHeading: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")),
		HelpCommand: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
	}
}

// StatusIcon returns a styled status icon for the given status string.
func (s Styles) StatusIcon(status string) string {
	switch status {
	case "pass", "ok":
		return s.Success.Render("✓")
	case "fail", "error":
		return s.Error.Render("✗")
	case "warn":
		return s.Warn.Render("⚠")
	case "skip":
		return s.Dim.Render("–")
	default:
		return s.Dim.Render("○")
	}
}

// PlatformHeader returns a styled platform section header.
func (s Styles) PlatformHeader(name string) string {
	return s.Header.Render(name + ":")
}
