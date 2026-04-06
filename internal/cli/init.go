package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/summon/internal/ui"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a new package",
	Long:  "Create a new summon package with summon.yaml and standard directory structure.",
	RunE:  runInit,
}

var (
	initName     string
	initPlatform []string
)

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "Package name (default: directory name)")
	initCmd.Flags().StringArrayVar(&initPlatform, "platform", nil, "Target platform(s)")
	rootCmd.AddCommand(initCmd)
}

// runInit scaffolds a new summon package in the current directory.
// It creates summon.yaml, standard subdirectories (skills/, agents/, commands/),
// and a README.md. If --name is not provided, the directory name is sanitized
// and used as the package name.
func runInit(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	name := initName
	if name == "" {
		// Derive a CLI-friendly name: lowercase, spaces/underscores → hyphens.
		name = filepath.Base(dir)
		name = strings.ToLower(name)
		name = strings.ReplaceAll(name, " ", "-")
		name = strings.ReplaceAll(name, "_", "-")
	}

	manifestPath := filepath.Join(dir, "summon.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		return fmt.Errorf("summon.yaml already exists in this directory")
	}

	m := map[string]interface{}{
		"name":        name,
		"version":     "0.1.0",
		"description": fmt.Sprintf("%s package", name),
	}
	if len(initPlatform) > 0 {
		m["platforms"] = initPlatform
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling summon.yaml: %w", err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return err
	}

	// Create conventional component directories if they don't already exist.
	dirs := []string{"skills", "agents", "commands"}
	for _, d := range dirs {
		p := filepath.Join(dir, d)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		}
	}

	readmePath := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		content := fmt.Sprintf("# %s\n\nA summon package.\n", name)
		if err := os.WriteFile(readmePath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	ui.Success("Initialized package %q", name)
	ui.Detail("Created: summon.yaml, skills/, agents/, commands/, README.md")
	return nil
}
