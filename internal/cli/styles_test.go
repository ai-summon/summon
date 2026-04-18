package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestNewStyles_NoColor_ProducesUnstyledOutput(t *testing.T) {
	s := NewStyles(true)

	// All text styles should render plain text (no ANSI codes)
	styles := map[string]lipgloss.Style{
		"Header":      s.Header,
		"Success":     s.Success,
		"Error":       s.Error,
		"Warn":        s.Warn,
		"Dim":         s.Dim,
		"Name":        s.Name,
		"URL":         s.URL,
		"Star":        s.Star,
		"Bullet":      s.Bullet,
		"HelpHeading": s.HelpHeading,
		"HelpCommand": s.HelpCommand,
	}
	for name, style := range styles {
		result := style.Render("test")
		assert.Equal(t, "test", result, "%s should produce plain text when noColor=true", name)
	}
}

func TestNewStyles_Color_StylesAreConfigured(t *testing.T) {
	s := NewStyles(false)
	sNoColor := NewStyles(true)

	// Verify each colored style differs from its noColor counterpart
	assert.NotEqual(t, sNoColor.Header, s.Header, "Header should differ from noColor")
	assert.NotEqual(t, sNoColor.Success, s.Success, "Success should differ from noColor")
	assert.NotEqual(t, sNoColor.Error, s.Error, "Error should differ from noColor")
	assert.NotEqual(t, sNoColor.Warn, s.Warn, "Warn should differ from noColor")
	assert.NotEqual(t, sNoColor.Dim, s.Dim, "Dim should differ from noColor")
	assert.NotEqual(t, sNoColor.Name, s.Name, "Name should differ from noColor")
	assert.NotEqual(t, sNoColor.URL, s.URL, "URL should differ from noColor")
	assert.NotEqual(t, sNoColor.Star, s.Star, "Star should differ from noColor")
	assert.NotEqual(t, sNoColor.Bullet, s.Bullet, "Bullet should differ from noColor")
	assert.NotEqual(t, sNoColor.HelpHeading, s.HelpHeading, "HelpHeading should differ from noColor")
	assert.NotEqual(t, sNoColor.HelpCommand, s.HelpCommand, "HelpCommand should differ from noColor")
}

func TestStatusIcon_AllMappings(t *testing.T) {
	s := NewStyles(true) // noColor for predictable assertions

	tests := []struct {
		status string
		icon   string
	}{
		{"pass", "✓"},
		{"ok", "✓"},
		{"fail", "✗"},
		{"error", "✗"},
		{"warn", "⚠"},
		{"skip", "–"},
		{"unknown", "○"},
		{"", "○"},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			result := s.StatusIcon(tc.status)
			assert.Equal(t, tc.icon, result)
		})
	}
}

func TestStatusIcon_Styled(t *testing.T) {
	s := NewStyles(false)

	// Verify icons contain the expected characters regardless of ANSI
	assert.Contains(t, s.StatusIcon("pass"), "✓")
	assert.Contains(t, s.StatusIcon("fail"), "✗")
	assert.Contains(t, s.StatusIcon("warn"), "⚠")
}

func TestPlatformHeader_NoColor(t *testing.T) {
	s := NewStyles(true)

	result := s.PlatformHeader("copilot")
	assert.Equal(t, "copilot:", result)

	result = s.PlatformHeader("claude")
	assert.Equal(t, "claude:", result)
}

func TestPlatformHeader_Styled(t *testing.T) {
	s := NewStyles(false)

	result := s.PlatformHeader("copilot")
	assert.Contains(t, result, "copilot:")
}

func TestNewStyles_Box_KeepsPaddingUnderNoColor(t *testing.T) {
	s := NewStyles(true)

	// Box should still have structural formatting (border/padding)
	result := s.Box.Render("content")
	// Box with border/padding produces multi-line output longer than plain text
	assert.True(t, len(result) > len("content"), "Box should preserve structural formatting under noColor")
	assert.True(t, strings.Contains(result, "content"), "Box should contain the text")
}
