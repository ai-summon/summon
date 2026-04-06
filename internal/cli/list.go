package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/user/summon/internal/installer"
	"github.com/user/summon/internal/registry"
	"github.com/user/summon/internal/store"
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		broken := ""
		if e.Broken {
			broken = " (broken link)"
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
