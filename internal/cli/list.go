package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/registry"
	"github.com/ai-summon/summon/internal/store"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed packages",
	RunE:  runList,
}

var (
	listGlobal  bool
	listProject bool
	listScope   string
	listJSON    bool
)

func init() {
	listCmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "Shortcut for --scope user")
	listCmd.Flags().BoolVarP(&listProject, "project", "p", false, "Shortcut for --scope project")
	listCmd.Flags().StringVar(&listScope, "scope", "", "Filter to one scope. One of local, project, user")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.MarkFlagsMutuallyExclusive("scope", "global", "project")
	rootCmd.AddCommand(listCmd)
}

// listEntry is the display/serialization model for "summon list".
type listEntry struct {
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Scope     string   `json:"scope"`
	Source    string   `json:"source"`
	Platforms []string `json:"platforms,omitempty"`
	Broken    bool     `json:"broken,omitempty"`
}

// runList prints every installed package in either tab-aligned text or JSON.
// It checks the store for broken symlinks and annotates them.
func runList(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	scopes, err := resolveQueryScopes(listScope, listGlobal, listProject)
	if err != nil {
		return err
	}

	var entries []listEntry
	for _, scope := range scopes {
		paths := installer.ResolvePaths(scope, projectDir)
		reg, err := registry.Load(paths.RegistryPath)
		if err != nil {
			return fmt.Errorf("loading %s registry: %w", scope.String(), err)
		}

		s := store.New(paths.StoreDir)
		for name, entry := range reg.Packages {
			src := entry.Source.URL
			if entry.Source.Type == "github" && entry.Source.Ref != "" {
				src = fmt.Sprintf("github:%s@%s", extractRepoPath(entry.Source.URL), entry.Source.Ref)
			}

			entries = append(entries, listEntry{
				Name:      name,
				Version:   entry.Version,
				Scope:     scope.String(),
				Source:    src,
				Platforms: entry.Platforms,
				Broken:    s.IsBrokenLink(name),
			})
		}
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stdout, "No packages installed.")
		return nil
	}

	if listJSON {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	headers := [4]string{"Package", "Version", "Scope", "Source"}

	// Compute column widths for the separator line.
	widths := [4]int{}
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, e := range entries {
		src := e.Source
		if e.Broken {
			src += " (broken)"
		}
		cols := [4]string{e.Name, e.Version, e.Scope, src}
		for i, c := range cols {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", headers[0], headers[1], headers[2], headers[3])
	seps := [4]string{}
	for i, width := range widths {
		seps[i] = strings.Repeat("─", width)
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", seps[0], seps[1], seps[2], seps[3])
	for _, e := range entries {
		broken := ""
		if e.Broken {
			broken = " (broken)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s%s\n", e.Name, e.Version, e.Scope, e.Source, broken)
	}
	return w.Flush()
}

// extractRepoPath strips the "https://github.com/" prefix from a URL,
// returning the "owner/repo" portion for compact display.
func extractRepoPath(url string) string {
	prefix := "https://github.com/"
	if len(url) > len(prefix) {
		return url[len(prefix):]
	}
	return url
}
