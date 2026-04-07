package installer

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// Stdout and Stderr are the writers used by status output functions.
// Tests can replace these to capture output.
var (
	Stdout io.Writer = os.Stdout
	Stderr io.Writer = os.Stderr
)

const statusLabelWidth = 12

const (
	boldGreen = "\033[1;32m"
	boldRed   = "\033[1;31m"
	boldYellow = "\033[1;33m"
	reset     = "\033[0m"
)

// colorLabel returns a map from label text to the ANSI color used for it.
func labelColor(label string) string {
	switch label {
	case "Error":
		return boldRed
	case "Warning":
		return boldYellow
	default:
		return boldGreen
	}
}

// isTerminal reports whether w is connected to a terminal.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// Status prints a right-aligned status label followed by a formatted message to Stdout.
// The label is bold green when writing to a terminal.
func Status(label, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if isTerminal(Stdout) {
		color := labelColor(label)
		fmt.Fprintf(Stdout, "%s%*s%s %s\n", color, statusLabelWidth, label, reset, msg)
	} else {
		fmt.Fprintf(Stdout, "%*s %s\n", statusLabelWidth, label, msg)
	}
}

// StatusErr prints a right-aligned status label followed by a formatted message to Stderr.
// The label is colored (yellow for warnings, red for errors) when writing to a terminal.
func StatusErr(label, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if isTerminal(Stderr) {
		color := labelColor(label)
		fmt.Fprintf(Stderr, "%s%*s%s %s\n", color, statusLabelWidth, label, reset, msg)
	} else {
		fmt.Fprintf(Stderr, "%*s %s\n", statusLabelWidth, label, msg)
	}
}
