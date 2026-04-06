package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestBold_ColorEnabled(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	got := Bold("hello")
	if !strings.Contains(got, "\033[1m") {
		t.Errorf("Bold with color on should contain ANSI bold code, got %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("Bold should contain the original text")
	}
}

func TestBold_ColorDisabled(t *testing.T) {
	SetColorEnabled(false)

	got := Bold("hello")
	if got != "hello" {
		t.Errorf("Bold with color off should return plain text, got %q", got)
	}
}

func TestDim_ColorEnabled(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	got := Dim("faded")
	if !strings.Contains(got, "\033[2m") {
		t.Errorf("Dim with color on should contain ANSI dim code, got %q", got)
	}
}

func TestDim_ColorDisabled(t *testing.T) {
	SetColorEnabled(false)

	got := Dim("faded")
	if got != "faded" {
		t.Errorf("Dim with color off should return plain text, got %q", got)
	}
}

func TestGreen_ColorEnabled(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	got := Green("ok")
	if !strings.Contains(got, "\033[32m") {
		t.Errorf("Green with color on should contain ANSI green code, got %q", got)
	}
}

func TestYellow_ColorEnabled(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	got := Yellow("caution")
	if !strings.Contains(got, "\033[33m") {
		t.Errorf("Yellow with color on should contain ANSI yellow code, got %q", got)
	}
}

func TestRed_ColorEnabled(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	got := Red("danger")
	if !strings.Contains(got, "\033[31m") {
		t.Errorf("Red with color on should contain ANSI red code, got %q", got)
	}
}

func TestCyan_ColorEnabled(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	got := Cyan("info")
	if !strings.Contains(got, "\033[36m") {
		t.Errorf("Cyan with color on should contain ANSI cyan code, got %q", got)
	}
}

func TestColorWrappers_Disabled(t *testing.T) {
	SetColorEnabled(false)

	cases := []struct {
		name string
		fn   func(string) string
		in   string
	}{
		{"Green", Green, "ok"},
		{"Yellow", Yellow, "caution"},
		{"Red", Red, "danger"},
		{"Cyan", Cyan, "info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.in)
			if got != tc.in {
				t.Errorf("%s with color off should return plain text, got %q", tc.name, got)
			}
		})
	}
}

// captureOutput captures what a function writes to the given file descriptor.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestInfo_NoColor(t *testing.T) {
	SetColorEnabled(false)

	out := captureStdout(t, func() {
		Info("Fetching %s...", "https://example.com")
	})
	if !strings.Contains(out, "→") {
		t.Errorf("Info should contain → symbol, got %q", out)
	}
	if !strings.Contains(out, "Fetching https://example.com...") {
		t.Errorf("Info should contain formatted message, got %q", out)
	}
	if strings.Contains(out, "\033[") {
		t.Errorf("Info with color off should not contain ANSI codes, got %q", out)
	}
}

func TestSuccess_NoColor(t *testing.T) {
	SetColorEnabled(false)

	out := captureStdout(t, func() {
		Success("Installed %s@%s", "pkg", "1.0.0")
	})
	if !strings.Contains(out, "✓") {
		t.Errorf("Success should contain ✓ symbol, got %q", out)
	}
	if !strings.Contains(out, "Installed pkg@1.0.0") {
		t.Errorf("Success should contain formatted message, got %q", out)
	}
}

func TestWarn_NoColor(t *testing.T) {
	SetColorEnabled(false)

	out := captureStderr(t, func() {
		Warn("something went wrong")
	})
	if !strings.Contains(out, "⚠") {
		t.Errorf("Warn should contain ⚠ symbol, got %q", out)
	}
	if !strings.Contains(out, "something went wrong") {
		t.Errorf("Warn should contain message, got %q", out)
	}
}

func TestError_NoColor(t *testing.T) {
	SetColorEnabled(false)

	out := captureStderr(t, func() {
		Error("failed to do %s", "thing")
	})
	if !strings.Contains(out, "✗") {
		t.Errorf("Error should contain ✗ symbol, got %q", out)
	}
	if !strings.Contains(out, "failed to do thing") {
		t.Errorf("Error should contain formatted message, got %q", out)
	}
}

func TestDetail_NoColor(t *testing.T) {
	SetColorEnabled(false)

	out := captureStdout(t, func() {
		Detail("Scope: %s", "user")
	})
	if !strings.HasPrefix(out, "  ") {
		t.Errorf("Detail should start with 2-space indent, got %q", out)
	}
	if !strings.Contains(out, "Scope: user") {
		t.Errorf("Detail should contain formatted message, got %q", out)
	}
}

func TestInfo_WithColor(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	out := captureStdout(t, func() {
		Info("test message")
	})
	if !strings.Contains(out, "\033[36m") {
		t.Errorf("Info with color should contain cyan ANSI code, got %q", out)
	}
	if !strings.Contains(out, "→") {
		t.Errorf("Info with color should contain → symbol, got %q", out)
	}
}

func TestSuccess_WithColor(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	out := captureStdout(t, func() {
		Success("done")
	})
	if !strings.Contains(out, "\033[32m") {
		t.Errorf("Success with color should contain green ANSI code, got %q", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("Success with color should contain ✓ symbol, got %q", out)
	}
}

func TestDetail_WithColor(t *testing.T) {
	SetColorEnabled(true)
	defer SetColorEnabled(false)

	out := captureStdout(t, func() {
		Detail("extra info")
	})
	if !strings.Contains(out, "\033[2m") {
		t.Errorf("Detail with color should contain dim ANSI code, got %q", out)
	}
	if !strings.HasPrefix(out, "  ") {
		t.Errorf("Detail should start with 2-space indent, got %q", out)
	}
}

func TestSetColorEnabled_Override(t *testing.T) {
	SetColorEnabled(true)
	if !IsColorEnabled() {
		t.Error("IsColorEnabled should return true after SetColorEnabled(true)")
	}

	SetColorEnabled(false)
	if IsColorEnabled() {
		t.Error("IsColorEnabled should return false after SetColorEnabled(false)")
	}
}
