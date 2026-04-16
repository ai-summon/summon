package cli

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/ai-summon/summon/internal/marketplace"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var marketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Manage marketplace registrations",
}

func init() {
	rootCmd.AddCommand(marketplaceCmd)

	addCmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Register a custom marketplace",
		Args:  cobra.ExactArgs(1),
		RunE:  runMarketplaceAdd,
	}

	listMktCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered marketplaces",
		RunE:  runMarketplaceList,
	}

	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a user-registered marketplace",
		Args:  cobra.ExactArgs(1),
		RunE:  runMarketplaceRemove,
	}

	browseCmd := &cobra.Command{
		Use:   "browse <name>",
		Short: "Browse packages available in a marketplace",
		Args:  cobra.ExactArgs(1),
		RunE:  runMarketplaceBrowse,
	}

	marketplaceCmd.AddCommand(addCmd, listMktCmd, removeCmd, browseCmd)
}

// --- marketplace add ---

type addDeps struct {
	stdout   io.Writer
	stderr   io.Writer
	adapters []platform.Adapter
}

func runMarketplaceAdd(cmd *cobra.Command, args []string) error {
	runner := &execRunner{}
	adapters := platform.DetectAdapters(runner)
	adapters, _ = platform.FilterByTarget(adapters, targetFlag)

	deps := &addDeps{
		stdout:   cmd.OutOrStdout(),
		stderr:   cmd.ErrOrStderr(),
		adapters: adapters,
	}
	return runMarketplaceAddWith(args[0], deps)
}

func runMarketplaceAddWith(source string, deps *addDeps) error {
	out := deps.stdout
	name := deriveMarketplaceName(source)

	if len(deps.adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected. Install copilot or claude CLI first")
	}

	fmt.Fprintf(out, "Registering marketplace %q (%s)\n", name, source)

	for _, a := range deps.adapters {
		if err := a.EnsureMarketplace(name, source); err != nil {
			fmt.Fprintf(deps.stderr, "⚠ %s: failed to register marketplace: %v\n", a.Name(), err)
		} else {
			fmt.Fprintf(out, "  ✓ %s: marketplace registered\n", a.Name())
		}
	}

	return nil
}

// --- marketplace list ---

type marketplaceListDeps struct {
	stdout   io.Writer
	noColor  bool
	adapters []platform.Adapter
}

func runMarketplaceList(cmd *cobra.Command, args []string) error {
	runner := &execRunner{}
	adapters := platform.DetectAdapters(runner)
	adapters, _ = platform.FilterByTarget(adapters, targetFlag)

	deps := &marketplaceListDeps{
		stdout:   cmd.OutOrStdout(),
		adapters: adapters,
	}
	return runMarketplaceListWith(deps)
}

func runMarketplaceListWith(deps *marketplaceListDeps) error {
	out := deps.stdout

	if len(deps.adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	starStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	bulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	badgeStyle := lipgloss.NewStyle().Faint(true)
	urlStyle := lipgloss.NewStyle().Faint(true)
	if deps.noColor {
		headerStyle = lipgloss.NewStyle()
		starStyle = lipgloss.NewStyle()
		bulletStyle = lipgloss.NewStyle()
		badgeStyle = lipgloss.NewStyle()
		urlStyle = lipgloss.NewStyle()
	}

	first := true
	for _, a := range deps.adapters {
		marketplaces, err := a.ListMarketplaces()
		if err != nil {
			continue
		}

		// Sort: official marketplace first, then alphabetical
		sort.Slice(marketplaces, func(i, j int) bool {
			iOfficial := marketplaces[i].Name == "summon-marketplace"
			jOfficial := marketplaces[j].Name == "summon-marketplace"
			if iOfficial != jOfficial {
				return iOfficial
			}
			return marketplaces[i].Name < marketplaces[j].Name
		})

		// Title-case the adapter name for the header
		title := strings.ToUpper(a.Name()[:1]) + a.Name()[1:]

		if !first {
			fmt.Fprintln(out)
		}
		first = false
		fmt.Fprintf(out, "\n%s\n", headerStyle.Render(fmt.Sprintf("%s (%d):", title, len(marketplaces))))

		for _, m := range marketplaces {
			icon := bulletStyle.Render("●")
			badge := ""
			if m.Name == "summon-marketplace" {
				icon = starStyle.Render("★")
				badge = "  " + badgeStyle.Render("official")
			}
			fmt.Fprintf(out, "  %s %s%s  %s\n", icon, m.Name, badge, urlStyle.Render(m.Source))
		}
	}

	return nil
}

// --- marketplace remove ---

type removeDeps struct {
	stdout   io.Writer
	stderr   io.Writer
	adapters []platform.Adapter
}

func runMarketplaceRemove(cmd *cobra.Command, args []string) error {
	runner := &execRunner{}
	adapters := platform.DetectAdapters(runner)
	adapters, _ = platform.FilterByTarget(adapters, targetFlag)

	deps := &removeDeps{
		stdout:   cmd.OutOrStdout(),
		stderr:   cmd.ErrOrStderr(),
		adapters: adapters,
	}
	return runMarketplaceRemoveWith(args[0], deps)
}

func runMarketplaceRemoveWith(name string, deps *removeDeps) error {
	out := deps.stdout

	if len(deps.adapters) == 0 {
		return fmt.Errorf("no supported CLIs detected")
	}

	if name == "summon-marketplace" {
		return fmt.Errorf("cannot remove the official marketplace")
	}

	fmt.Fprintf(out, "Removing marketplace %q\n", name)

	for _, a := range deps.adapters {
		if err := a.RemoveMarketplace(name); err != nil {
			fmt.Fprintf(deps.stderr, "⚠ %s: %v\n", a.Name(), err)
		} else {
			fmt.Fprintf(out, "  ✓ %s: marketplace removed\n", a.Name())
		}
	}

	return nil
}

// --- marketplace browse ---

type browseDeps struct {
	stdout      io.Writer
	noColor     bool
	adapters    []platform.Adapter
	localReader func(name string) (marketplace.Index, error)
	fetcher     marketplace.IndexFetcher
}

func runMarketplaceBrowse(cmd *cobra.Command, args []string) error {
	runner := &execRunner{}
	adapters := platform.DetectAdapters(runner)
	adapters, _ = platform.FilterByTarget(adapters, targetFlag)

	deps := &browseDeps{
		stdout:      cmd.OutOrStdout(),
		adapters:    adapters,
		localReader: marketplace.ReadLocalIndex,
		fetcher:     marketplace.NewDefaultIndexFetcher(&http.Client{}, &execGitRunner{}),
	}
	return runMarketplaceBrowseWith(args[0], deps)
}

func runMarketplaceBrowseWith(name string, deps *browseDeps) error {
	out := deps.stdout

	// Try local cache first
	idx, err := deps.localReader(name)
	if err != nil {
		// Fall back to remote fetch — resolve source from adapters
		source, sourceErr := resolveMarketplaceSourceFromAdapters(name, deps.adapters)
		if sourceErr != nil {
			return sourceErr
		}
		idx, err = deps.fetcher.FetchMarketplaceIndex(source)
		if err != nil {
			return fmt.Errorf("failed to fetch marketplace index: %w", err)
		}
	}

	if len(idx) == 0 {
		fmt.Fprintf(out, "No packages found in %s\n", name)
		return nil
	}

	// Collect and sort package names
	names := make([]string, 0, len(idx))
	for n := range idx {
		names = append(names, n)
	}
	sort.Strings(names)

	// Find longest name for column alignment
	maxLen := 0
	for _, n := range names {
		if len(n) > maxLen {
			maxLen = len(n)
		}
	}

	// Styled output
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	descStyle := lipgloss.NewStyle().Faint(true)
	if deps.noColor {
		headerStyle = lipgloss.NewStyle()
		nameStyle = lipgloss.NewStyle()
		descStyle = lipgloss.NewStyle()
	}

	fmt.Fprintf(out, "\n%s\n\n", headerStyle.Render(fmt.Sprintf("Packages in %s:", name)))
	for _, n := range names {
		entry := idx[n]
		padding := strings.Repeat(" ", maxLen-len(n)+4)
		fmt.Fprintf(out, "  %s%s%s\n", nameStyle.Render(n), padding, descStyle.Render(entry.Description))
	}
	fmt.Fprintf(out, "\n%d package(s) available\n", len(names))

	return nil
}

// resolveMarketplaceSourceFromAdapters resolves a marketplace name to its source URL
// by querying native CLIs.
func resolveMarketplaceSourceFromAdapters(name string, adapters []platform.Adapter) (string, error) {
	if name == "summon-marketplace" {
		return marketplace.OfficialMarketplaceURL, nil
	}

	for _, a := range adapters {
		marketplaces, err := a.ListMarketplaces()
		if err != nil {
			continue
		}
		for _, m := range marketplaces {
			if m.Name == name {
				return m.Source, nil
			}
		}
	}

	return "", fmt.Errorf("marketplace %q not found. Use 'summon marketplace add' to register it", name)
}

// --- helpers ---

func deriveMarketplaceName(source string) string {
	source = strings.TrimSuffix(source, ".git")
	parts := strings.Split(strings.TrimRight(source, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return source
}
