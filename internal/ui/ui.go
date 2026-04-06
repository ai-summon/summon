// Package ui provides styled terminal output for the summon CLI.
// It uses ANSI escape codes for color and supports graceful degradation
// when output is piped or the NO_COLOR environment variable is set.
package ui

import (
	"fmt"
	"os"
	"sync"
)

// ANSI escape codes.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
)

// Unicode symbols used as line prefixes.
const (
	symbolInfo    = "→"
	symbolSuccess = "✓"
	symbolWarn    = "⚠"
	symbolError   = "✗"
)

var (
	colorEnabled bool
	initOnce     sync.Once
)

func detectColor() {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		colorEnabled = false
		return
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		colorEnabled = false
		return
	}
	colorEnabled = info.Mode()&os.ModeCharDevice != 0
}

func ensureInit() {
	initOnce.Do(detectColor)
}

// SetColorEnabled overrides automatic color detection.
func SetColorEnabled(enabled bool) {
	initOnce.Do(func() {}) // mark as done so detectColor doesn't run
	colorEnabled = enabled
}

// IsColorEnabled reports whether color output is active.
func IsColorEnabled() bool {
	ensureInit()
	return colorEnabled
}

// Bold wraps s in bold ANSI codes when color is enabled.
func Bold(s string) string {
	ensureInit()
	if !colorEnabled {
		return s
	}
	return bold + s + reset
}

// Dim wraps s in dim ANSI codes when color is enabled.
func Dim(s string) string {
	ensureInit()
	if !colorEnabled {
		return s
	}
	return dim + s + reset
}

// Green wraps s in green ANSI codes when color is enabled.
func Green(s string) string {
	ensureInit()
	if !colorEnabled {
		return s
	}
	return green + s + reset
}

// Yellow wraps s in yellow ANSI codes when color is enabled.
func Yellow(s string) string {
	ensureInit()
	if !colorEnabled {
		return s
	}
	return yellow + s + reset
}

// Red wraps s in red ANSI codes when color is enabled.
func Red(s string) string {
	ensureInit()
	if !colorEnabled {
		return s
	}
	return red + s + reset
}

// Cyan wraps s in cyan ANSI codes when color is enabled.
func Cyan(s string) string {
	ensureInit()
	if !colorEnabled {
		return s
	}
	return cyan + s + reset
}

// Info prints an in-progress/informational message to stdout with a cyan "→" prefix.
func Info(format string, args ...interface{}) {
	ensureInit()
	msg := fmt.Sprintf(format, args...)
	if colorEnabled {
		fmt.Fprintf(os.Stdout, "%s%s%s %s\n", cyan, symbolInfo, reset, msg)
	} else {
		fmt.Fprintf(os.Stdout, "%s %s\n", symbolInfo, msg)
	}
}

// Success prints a success message to stdout with a green "✓" prefix.
func Success(format string, args ...interface{}) {
	ensureInit()
	msg := fmt.Sprintf(format, args...)
	if colorEnabled {
		fmt.Fprintf(os.Stdout, "%s%s%s %s\n", green, symbolSuccess, reset, msg)
	} else {
		fmt.Fprintf(os.Stdout, "%s %s\n", symbolSuccess, msg)
	}
}

// Warn prints a warning message to stderr with a yellow "⚠" prefix.
func Warn(format string, args ...interface{}) {
	ensureInit()
	msg := fmt.Sprintf(format, args...)
	if colorEnabled {
		fmt.Fprintf(os.Stderr, "%s%s%s %s\n", yellow, symbolWarn, reset, msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", symbolWarn, msg)
	}
}

// Error prints an error message to stderr with a red "✗" prefix.
func Error(format string, args ...interface{}) {
	ensureInit()
	msg := fmt.Sprintf(format, args...)
	if colorEnabled {
		fmt.Fprintf(os.Stderr, "%s%s%s %s\n", red, symbolError, reset, msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", symbolError, msg)
	}
}

// Detail prints a secondary/detail message to stdout with a 2-space indent.
func Detail(format string, args ...interface{}) {
	ensureInit()
	msg := fmt.Sprintf(format, args...)
	if colorEnabled {
		fmt.Fprintf(os.Stdout, "  %s%s%s\n", dim, msg, reset)
	} else {
		fmt.Fprintf(os.Stdout, "  %s\n", msg)
	}
}
