//go:build !windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveProfilePath returns the shell profile file that the installer would
// have modified, using the same logic as install.sh's update_path_if_needed.
func resolveProfilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	shell := os.Getenv("SHELL")
	switch {
	case strings.HasSuffix(shell, "/zsh"):
		return filepath.Join(home, ".zprofile"), nil
	case strings.HasSuffix(shell, "/bash"):
		return filepath.Join(home, ".bashrc"), nil
	default:
		return filepath.Join(home, ".profile"), nil
	}
}

// pathExportLine returns the exact PATH export line the installer writes.
func pathExportLine(binaryPath string) string {
	binDir := filepath.Dir(binaryPath)
	return fmt.Sprintf(`export PATH="%s:$PATH"`, binDir)
}

// cleanPath removes the installer's PATH export line from the shell profile.
func cleanPath(binaryPath string) error {
	profilePath, err := resolveProfilePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no profile file, nothing to clean
		}
		return fmt.Errorf("cannot read %s: %w", profilePath, err)
	}

	line := pathExportLine(binaryPath)
	content := string(data)
	if !containsLine(content, line) {
		return nil // line not found, no-op
	}

	content = removeLine(content, line)

	// Write back atomically: temp file + rename
	dir := filepath.Dir(profilePath)
	tmp, err := os.CreateTemp(dir, ".summon-uninstall-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cannot write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cannot close temp file: %w", err)
	}

	// Preserve original file permissions
	info, err := os.Stat(profilePath)
	if err == nil {
		os.Chmod(tmpName, info.Mode())
	}

	if err := os.Rename(tmpName, profilePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cannot update %s: %w", profilePath, err)
	}
	return nil
}

// containsLine checks if the content contains the exact line.
func containsLine(content, line string) bool {
	return strings.Contains(content, line)
}

// removeLine removes all occurrences of the line from the content,
// also cleaning up leading blank lines added by the installer.
func removeLine(content, line string) string {
	// Remove the exact line and the leading blank line the installer adds.
	// The installer writes: printf '\n%s\n' "$line" >> "$profile"
	// So the pattern is \n<line>\n
	content = strings.ReplaceAll(content, "\n"+line+"\n", "\n")
	// Handle case where the line is the last thing in the file (no trailing newline)
	content = strings.ReplaceAll(content, "\n"+line, "")
	// Handle case where the line is the first thing in the file
	content = strings.ReplaceAll(content, line+"\n", "")
	// Handle case where it's the only content
	content = strings.ReplaceAll(content, line, "")
	return content
}

// removeBinary removes the summon binary. On Unix, this is safe because the
// OS keeps the inode open until the running process exits.
// dataDir and keepData are unused on Unix (data dir is removed directly).
func removeBinary(binaryPath, _ string, _ bool) error {
	return os.Remove(binaryPath)
}

// dataDirRemovedByGC returns false on Unix — the main process removes it directly.
func dataDirRemovedByGC() bool {
	return false
}

// pathCleanupDescription returns a human-readable description of what PATH
// cleanup will do, for the removal plan display.
func pathCleanupDescription() string {
	profilePath, err := resolveProfilePath()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("PATH entry in: %s", profilePath)
}

// pathCleanupSuccessMessage returns the success message for PATH cleanup.
func pathCleanupSuccessMessage() string {
	profilePath, err := resolveProfilePath()
	if err != nil {
		return "Removed PATH entry"
	}
	return fmt.Sprintf("Removed PATH entry from %s", profilePath)
}

// binaryRemovalSuccessMessage returns the success message for binary removal.
func binaryRemovalSuccessMessage(binaryPath string) string {
	return fmt.Sprintf("Removed binary %s", binaryPath)
}
