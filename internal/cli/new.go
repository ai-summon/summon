package cli

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

//go:embed templates/*
var templates embed.FS

var newCmd = &cobra.Command{
	Use:   "new [flags] <PATH>",
	Short: "Create a new plugin project",
	Long:  "Create a new plugin project with scaffolding. Supports various plugin types (skill, agent, command, hook, mcp, generic).",
	Example: `  summon new my-plugin
  summon new --type skill my-skill
  summon new --type agent --name "My Agent" my-agent
  summon new --vcs none my-plugin
  summon new --help`,
	Args: cobra.ExactArgs(1),
	RunE: runNew,
}

var (
	newType string
	newName string
	newVCS  string
)

func init() {
	newCmd.Flags().StringVar(&newType, "type", "", "Plugin type: skill, agent, command, hook, mcp, generic (default: agent + skill)")
	newCmd.Flags().StringVar(&newName, "name", "", "Custom display name for the plugin (defaults to normalized path)")
	newCmd.Flags().StringVar(&newVCS, "vcs", "git", "Version control system: git, none")
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	projectPath := args[0]

	// Set default type if not specified
	if newType == "" {
		newType = "default"
	}

	// Validate type
	validTypes := []string{"skill", "agent", "command", "hook", "mcp", "generic", "default"}
	if !isValidType(newType, validTypes) {
		return fmt.Errorf("invalid plugin type '%s'. Valid types: %s", newType, strings.Join(validTypes, ", "))
	}

	// Validate VCS option
	validVCS := []string{"git", "none"}
	if !isValidVCS(newVCS, validVCS) {
		return fmt.Errorf("invalid vcs option '%s'. Valid options: %s", newVCS, strings.Join(validVCS, ", "))
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if directory exists
	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("directory already exists: %s", absPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check directory: %w", err)
	}

	// Normalize plugin name
	pluginName := newName
	if pluginName == "" {
		pluginName = normalizePluginName(filepath.Base(absPath))
	}

	// Create project structure
	if err := createProjectStructure(absPath, newType); err != nil {
		// Clean up on failure
		os.RemoveAll(absPath)
		return err
	}

	// Generate manifest file
	if err := generateManifest(absPath, pluginName); err != nil {
		os.RemoveAll(absPath)
		return err
	}

	// Copy templates
	if err := copyTemplates(absPath, newType, pluginName); err != nil {
		os.RemoveAll(absPath)
		return err
	}

	// Create starter files for type-specific folders
	if err := createStarterFiles(absPath, newType, pluginName); err != nil {
		os.RemoveAll(absPath)
		return err
	}

	// Copy .gitignore
	if err := copyGitignore(absPath); err != nil {
		os.RemoveAll(absPath)
		return err
	}

	// Initialize version control if requested
	if newVCS == "git" {
		if err := initializeGit(absPath); err != nil {
			// Warn but don't fail if git is unavailable
			fmt.Fprintf(os.Stderr, "Warning: Could not initialize git repository: %v\n", err)
		}
	}

	// Use ANSI color codes: green for action verbs
	green := "\033[32m"
	reset := "\033[0m"

	// Format the output based on type
	var typeLabel string
	if newType == "default" {
		typeLabel = "agent + skill"
	} else {
		typeLabel = newType
	}
	fmt.Printf("%s   Creating%s summon package `%s` (%s)\n", green, reset, pluginName, typeLabel)
	fmt.Printf("note: Run `summon install --path .` to install plugins in this package\n")

	return nil
}

// isValidType checks if the provided type is in the list of valid types
func isValidType(typeVal string, validTypes []string) bool {
	for _, vt := range validTypes {
		if typeVal == vt {
			return true
		}
	}
	return false
}

// isValidVCS checks if the provided VCS option is valid
func isValidVCS(vcsVal string, validVCS []string) bool {
	for _, vcs := range validVCS {
		if vcsVal == vcs {
			return true
		}
	}
	return false
}

// normalizePluginName converts a path/name to kebab-case
func normalizePluginName(name string) string {
	// Remove file extensions
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace underscores and spaces with hyphens
	name = strings.NewReplacer(
		"_", "-",
		" ", "-",
	).Replace(name)

	return name
}

// createProjectStructure creates the directory structure for the plugin type
func createProjectStructure(projectPath string, pluginType string) error {
	// Create root directory
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create type-specific subdirectories for markdown files
	var typeDirs []string
	switch pluginType {
	case "skill":
		typeDirs = []string{"skills"}
	case "agent":
		typeDirs = []string{"agents"}
	case "command":
		typeDirs = []string{"commands"}
	case "hook":
		typeDirs = []string{"hooks"}
	case "mcp":
		typeDirs = []string{"mcp"}
	case "default":
		// Default: create both agents and skills folders
		typeDirs = []string{"agents", "skills"}
	default:
		// Generic type has no subdirectories
		return nil
	}

	for _, typeDir := range typeDirs {
		typePath := filepath.Join(projectPath, typeDir)
		if err := os.MkdirAll(typePath, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	return nil
}

// generateManifest creates the .claude-plugin/plugin.json manifest file.
func generateManifest(projectPath, pluginName string) error {
	pluginDir := filepath.Join(projectPath, ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude-plugin dir: %w", err)
	}

	pluginJSON := fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "description": "A %s plugin for Summon",
  "author": %q
}
`, pluginName, pluginName, getAuthorName())

	pluginPath := filepath.Join(pluginDir, "plugin.json")
	if err := os.WriteFile(pluginPath, []byte(pluginJSON), 0o644); err != nil {
		return fmt.Errorf("failed to write plugin.json: %w", err)
	}

	return nil
}

// copyTemplates copies template files to the project
func copyTemplates(projectPath, pluginType, pluginName string) error {
	// Determine which README template to use
	var templatePath string

	switch pluginType {
	case "skill", "agent", "command", "hook", "mcp":
		templatePath = "templates/" + pluginType + "/README.md"
	default:
		templatePath = "templates/generic/README.md"
	}

	// Read README template from embedded filesystem
	readmeContent, err := templates.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read README template: %w", err)
	}

	// Substitute template variables
	content := string(readmeContent)
	content = strings.ReplaceAll(content, "{{NAME}}", pluginName)
	content = strings.ReplaceAll(content, "{{DESCRIPTION}}", fmt.Sprintf("A %s plugin for Summon", pluginName))
	content = strings.ReplaceAll(content, "{{YEAR}}", fmt.Sprintf("%d", time.Now().Year()))

	// Write README file
	readmeOutPath := filepath.Join(projectPath, "README.md")
	if err := os.WriteFile(readmeOutPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write README file: %w", err)
	}

	return nil
}

// createStarterFiles creates type-specific starter files in the type folders
func createStarterFiles(projectPath, pluginType, pluginName string) error {
	// Define files to create based on type
	type fileSpec struct {
		templatePath string
		outputName   string
		outputDir    string
		isSkill      bool // skills are in subdirectories
	}

	var files []fileSpec

	switch pluginType {
	case "agent":
		files = []fileSpec{
			{
				templatePath: "templates/agent/agent.yaml",
				outputName:   pluginName + ".md",
				outputDir:    "agents",
				isSkill:      false,
			},
		}

	case "skill":
		files = []fileSpec{
			{
				templatePath: "templates/skill/skill.yaml",
				outputName:   "SKILL.md",
				outputDir:    filepath.Join("skills", pluginName),
				isSkill:      true,
			},
		}

	case "command":
		files = []fileSpec{
			{
				templatePath: "templates/command/command.py",
				outputName:   pluginName + ".md",
				outputDir:    "commands",
				isSkill:      false,
			},
		}

	case "hook":
		files = []fileSpec{
			{
				templatePath: "templates/hook/hook.py",
				outputName:   pluginName + ".md",
				outputDir:    "hooks",
				isSkill:      false,
			},
		}

	case "mcp":
		files = []fileSpec{
			{
				templatePath: "templates/mcp/server.py",
				outputName:   "server.md",
				outputDir:    "mcp",
				isSkill:      false,
			},
		}

	case "default":
		// Default: create both agent and skill starter files
		files = []fileSpec{
			{
				templatePath: "templates/agent/agent.yaml",
				outputName:   pluginName + ".md",
				outputDir:    "agents",
				isSkill:      false,
			},
			{
				templatePath: "templates/skill/skill.yaml",
				outputName:   "SKILL.md",
				outputDir:    filepath.Join("skills", pluginName),
				isSkill:      true,
			},
		}

	default:
		// Generic type has no starter files
		return nil
	}

	// Create each file
	for _, file := range files {
		// For skills, create the subdirectory
		if file.isSkill {
			skillDir := filepath.Join(projectPath, file.outputDir)
			if err := os.MkdirAll(skillDir, 0755); err != nil {
				return fmt.Errorf("failed to create skill directory: %w", err)
			}
		}

		// Read template from embedded filesystem
		fileContent, err := templates.ReadFile(file.templatePath)
		if err != nil {
			return fmt.Errorf("failed to read starter template: %w", err)
		}

		// Substitute template variables
		content := string(fileContent)
		content = strings.ReplaceAll(content, "{{NAME}}", pluginName)
		content = strings.ReplaceAll(content, "{{DESCRIPTION}}", fmt.Sprintf("A %s package for Summon", pluginName))
		content = strings.ReplaceAll(content, "{{AUTHOR}}", getAuthorName())
		content = strings.ReplaceAll(content, "{{YEAR}}", fmt.Sprintf("%d", time.Now().Year()))

		// Write starter file
		outputPath := filepath.Join(projectPath, file.outputDir, file.outputName)
		if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write starter file: %w", err)
		}
	}

	return nil
}

// copyGitignore copies the .gitignore template to the project
func copyGitignore(projectPath string) error {
	// Read .gitignore template from embedded filesystem
	content, err := templates.ReadFile("templates/.gitignore")
	if err != nil {
		return fmt.Errorf("failed to read .gitignore template: %w", err)
	}

	outPath := filepath.Join(projectPath, ".gitignore")
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore file: %w", err)
	}

	return nil
}

// initializeGit initializes a git repository in the project
func initializeGit(projectPath string) error {
	// Initialize git repository
	initCmd := exec.Command("git", "init")
	initCmd.Dir = projectPath
	if _, err := initCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Add all files to git
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = projectPath
	if _, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Create initial commit
	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = projectPath
	if _, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}

// getAuthorName returns the system username for use as default author
func getAuthorName() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}
