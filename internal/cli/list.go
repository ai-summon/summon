package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/user/summon/internal/installer"
	"github.com/user/summon/internal/registry"
	"github.com/user/summon/internal/store"
	"github.com/user/summon/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed packages",
	RunE:  runList,
}

var (
	listGlobal bool
	listScope  string
	listJSON   bool
)

func init() {
	listCmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "List global packages only")
	listCmd.Flags().StringVar(&listScope, "scope", "", "Filter to one scope: user, project, or local")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(listCmd)
}

// listEntry is the display/serialization model for "summon list".
type listEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Scope   string `json:"scope"`
	Source  string `json:"source"`
	Broken  bool   `json:"broken,omitempty"`
}

// runList prints every installed package in either tab-aligned text or JSON.
// It checks the store for broken symlinks and annotates them.
func runList(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	scopes, err := resolveQueryScopes(listScope, listGlobal)
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
				Name:    name,
				Version: entry.Version,
				Scope:   scope.String(),
				Source:  src,
				Broken:  s.IsBrokenLink(name),
			})
		}
	}

	if len(entries) == 0 {
		ui.Info("No packages installed.")
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

	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tVERSION\tSCOPE\tSOURCE\n")
	for _, e := range entries {
		broken := ""
		if e.Broken {
			broken = " (broken link)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s%s\n", e.Name, e.Version, e.Scope, e.Source, broken)
	}
	w.Flush()

	lines := strings.Split(buf.String(), "\n")
	if len(lines) == 0 {
		return nil
	}

	header := lines[0]
	fmt.Fprintln(os.Stdout, ui.Bold(header))

	// Print separator matching header width.
	sepLen := len(header)
	sep := strings.Repeat("─", sepLen)
	fmt.Fprintln(os.Stdout, ui.Dim(sep))

	// Find column start positions from header.
	colStarts := []int{0}
	for _, col := range []string{"VERSION", "SCOPE", "SOURCE"} {
		idx := strings.Index(header, col)
		if idx >= 0 {
			colStarts = append(colStarts, idx)
		}
	}

	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		fmt.Fprintln(os.Stdout, colorizeListRow(line, colStarts))
	}
	return nil
}

// colorizeListRow applies per-column colors to an aligned list row.
// Column order: NAME (bold), VERSION (green), SCOPE (cyan), SOURCE (dim).
func colorizeListRow(line string, colStarts []int) string {
	cols := splitAtPositions(line, colStarts)
	// Apply styling per column.
	colorFns := []func(string) string{ui.Bold, ui.Green, ui.Cyan, ui.Dim}
	var parts []string
	for i, col := range cols {
		trimmed := strings.TrimRight(col, " ")
		padding := col[len(trimmed):]
		if i < len(colorFns) {
			// Special-case broken link in source column.
			if strings.Contains(trimmed, "(broken link)") {
				trimmed = strings.Replace(trimmed, "(broken link)", ui.Red("(broken link)"), 1)
				parts = append(parts, trimmed+padding)
			} else {
				parts = append(parts, colorFns[i](trimmed)+padding)
			}
		} else {
			parts = append(parts, col)
		}
	}
	return strings.Join(parts, "")
}

// splitAtPositions splits a line into segments at the given byte positions.
func splitAtPositions(line string, positions []int) []string {
	var result []string
	for i, pos := range positions {
		if pos >= len(line) {
			break
		}
		end := len(line)
		if i+1 < len(positions) && positions[i+1] < len(line) {
			end = positions[i+1]
		}
		result = append(result, line[pos:end])
	}
	return result
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
