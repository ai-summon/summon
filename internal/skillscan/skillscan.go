package skillscan

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillEntry represents a single skill found in a plugin.
type SkillEntry struct {
	Name        string // from SKILL.md frontmatter name field
	PluginName  string // owning plugin name
	Marketplace string // marketplace the plugin belongs to
	FilePath    string // relative path from plugin root to SKILL.md
	Order       int    // install precedence (lower = loaded first)
}

// Collision represents a skill name that appears in multiple locations.
type Collision struct {
	SkillName string
	Entries   []SkillEntry // sorted by Order (first = winner among plugins)
}

// ScanError records a non-fatal error encountered while scanning a plugin.
type ScanError struct {
	PluginName string
	Detail     string
	Err        error
}

func (e ScanError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.PluginName, e.Detail, e.Err)
}

// ParseSkillName extracts the "name" field from SKILL.md YAML frontmatter.
// Frontmatter is delimited by lines containing exactly "---".
func ParseSkillName(data []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))

	// First line must be "---"
	if !scanner.Scan() {
		return "", fmt.Errorf("empty file")
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return "", fmt.Errorf("missing frontmatter delimiter")
	}

	// Collect lines until closing "---"
	var fmLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}

	if len(fmLines) == 0 {
		return "", fmt.Errorf("empty frontmatter")
	}

	var fm struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
		return "", fmt.Errorf("invalid frontmatter YAML: %w", err)
	}

	if fm.Name == "" {
		return "", fmt.Errorf("frontmatter missing 'name' field")
	}

	return fm.Name, nil
}

// pluginManifestPaths lists the locations where plugin.json may be found,
// in the order defined by the Copilot CLI docs.
var pluginManifestPaths = []string{
	filepath.Join(".plugin", "plugin.json"),
	"plugin.json",
	filepath.Join(".github", "plugin", "plugin.json"),
	filepath.Join(".claude-plugin", "plugin.json"),
}

// FindPluginManifest returns the path to plugin.json within a plugin directory.
// It checks known locations in precedence order.
func FindPluginManifest(pluginDir string) (string, error) {
	for _, rel := range pluginManifestPaths {
		p := filepath.Join(pluginDir, rel)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no plugin manifest found in %s", pluginDir)
}

// pluginJSON is the minimal shape of plugin.json we need for skill path resolution.
type pluginJSON struct {
	Skills json.RawMessage `json:"skills"`
}

// ReadSkillDirs returns the absolute skill directory paths for a plugin.
// It reads the "skills" field from plugin.json (string or []string).
// Falls back to ["skills/"] if the manifest is absent or doesn't specify skills.
func ReadSkillDirs(pluginDir string) ([]string, error) {
	return readSkillDirsInternal(pluginDir, os.ReadFile)
}

type readFileFunc func(string) ([]byte, error)

func readSkillDirsInternal(pluginDir string, readFile readFileFunc) ([]string, error) {
	defaultDir := filepath.Join(pluginDir, "skills")

	manifestPath, err := FindPluginManifest(pluginDir)
	if err != nil {
		// No manifest — use default
		return []string{defaultDir}, nil
	}

	data, err := readFile(manifestPath)
	if err != nil {
		return []string{defaultDir}, nil
	}

	var pj pluginJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return []string{defaultDir}, nil
	}

	if len(pj.Skills) == 0 || string(pj.Skills) == "null" {
		return []string{defaultDir}, nil
	}

	// Try string first
	var single string
	if err := json.Unmarshal(pj.Skills, &single); err == nil {
		return []string{filepath.Join(pluginDir, single)}, nil
	}

	// Try string array
	var multiple []string
	if err := json.Unmarshal(pj.Skills, &multiple); err == nil {
		dirs := make([]string, len(multiple))
		for i, s := range multiple {
			dirs[i] = filepath.Join(pluginDir, s)
		}
		return dirs, nil
	}

	return []string{defaultDir}, nil
}

// ScanPlugin scans a single plugin directory for all SKILL.md files and returns
// a SkillEntry for each skill found. Non-fatal errors are returned separately.
func ScanPlugin(pluginDir, pluginName, marketplace string, order int) ([]SkillEntry, []ScanError) {
	skillDirs, err := ReadSkillDirs(pluginDir)
	if err != nil {
		return nil, []ScanError{{PluginName: pluginName, Detail: "reading skill dirs", Err: err}}
	}

	var entries []SkillEntry
	var scanErrors []ScanError

	for _, dir := range skillDirs {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				scanErrors = append(scanErrors, ScanError{
					PluginName: pluginName,
					Detail:     fmt.Sprintf("reading skill directory %s", dir),
					Err:        err,
				})
			}
			continue
		}

		for _, de := range dirEntries {
			if !de.IsDir() {
				continue
			}

			skillMD := filepath.Join(dir, de.Name(), "SKILL.md")
			data, err := os.ReadFile(skillMD)
			if err != nil {
				if !os.IsNotExist(err) {
					scanErrors = append(scanErrors, ScanError{
						PluginName: pluginName,
						Detail:     fmt.Sprintf("reading %s", skillMD),
						Err:        err,
					})
				}
				continue
			}

			name, err := ParseSkillName(data)
			if err != nil {
				scanErrors = append(scanErrors, ScanError{
					PluginName: pluginName,
					Detail:     fmt.Sprintf("parsing %s", skillMD),
					Err:        err,
				})
				continue
			}

			// Compute relative path from plugin root
			relPath, _ := filepath.Rel(pluginDir, skillMD)
			if relPath == "" {
				relPath = skillMD
			}

			entries = append(entries, SkillEntry{
				Name:        name,
				PluginName:  pluginName,
				Marketplace: marketplace,
				FilePath:    relPath,
				Order:       order,
			})
		}
	}

	return entries, scanErrors
}

// DetectCollisions groups skills by name and returns groups with 2+ entries.
// Entries within each collision are sorted by Order (ascending = higher precedence first).
func DetectCollisions(entries []SkillEntry) []Collision {
	groups := make(map[string][]SkillEntry)
	for _, e := range entries {
		groups[e.Name] = append(groups[e.Name], e)
	}

	var collisions []Collision
	for name, group := range groups {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].Order != group[j].Order {
				return group[i].Order < group[j].Order
			}
			return group[i].PluginName < group[j].PluginName
		})
		collisions = append(collisions, Collision{
			SkillName: name,
			Entries:   group,
		})
	}

	// Sort collisions by skill name for deterministic output
	sort.Slice(collisions, func(i, j int) bool {
		return collisions[i].SkillName < collisions[j].SkillName
	})

	return collisions
}
