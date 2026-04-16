package cli

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/ai-summon/summon/internal/marketplace"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var configPath string

var marketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Manage marketplace registrations",
}

func init() {
	rootCmd.AddCommand(marketplaceCmd)

	// Add subcommands
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

func getConfigPath() string {
	if configPath != "" {
		return configPath
	}
	return marketplace.DefaultConfigPath()
}

func runMarketplaceAdd(cmd *cobra.Command, args []string) error {
	source := args[0]
	out := cmd.OutOrStdout()

	// Derive name from source
	name := deriveMarketplaceName(source)

	cfg, err := marketplace.LoadConfig(getConfigPath())
	if err != nil {
		return err
	}

	if err := cfg.AddMarketplace(name, source); err != nil {
		return err
	}

	if err := marketplace.SaveConfig(getConfigPath(), cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "Marketplace %q registered (%s)\n", name, source)
	return nil
}

type marketplaceListDeps struct {
	stdout  io.Writer
	noColor bool
}

func runMarketplaceList(cmd *cobra.Command, args []string) error {
	deps := &marketplaceListDeps{
		stdout: cmd.OutOrStdout(),
	}
	return runMarketplaceListWith(deps)
}

func runMarketplaceListWith(deps *marketplaceListDeps) error {
	out := deps.stdout

	cfg, err := marketplace.LoadConfig(getConfigPath())
	if err != nil {
		return err
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

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", headerStyle.Render("Marketplaces:"))

	// Official marketplace
	fmt.Fprintf(out, "\n  %s %s  %s\n", starStyle.Render("★"), "summon-marketplace", badgeStyle.Render("official"))
	fmt.Fprintf(out, "    %s\n", urlStyle.Render(marketplace.OfficialMarketplaceURL))

	// User-registered marketplaces
	for _, m := range cfg.Marketplaces {
		fmt.Fprintf(out, "\n  %s %s\n", bulletStyle.Render("●"), m.Name)
		fmt.Fprintf(out, "    %s\n", urlStyle.Render(m.Source))
	}

	fmt.Fprintf(out, "\n%d marketplace(s) registered\n", 1+len(cfg.Marketplaces))
	return nil
}

func runMarketplaceRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	out := cmd.OutOrStdout()

	cfg, err := marketplace.LoadConfig(getConfigPath())
	if err != nil {
		return err
	}

	if err := cfg.RemoveMarketplace(name); err != nil {
		return err
	}

	if err := marketplace.SaveConfig(getConfigPath(), cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "Marketplace %q removed\n", name)
	return nil
}

// browseDeps holds injectable dependencies for the browse command.
type browseDeps struct {
	stdout     io.Writer
	noColor    bool
	configPath string
	localReader func(name string) (marketplace.Index, error)
	fetcher     marketplace.IndexFetcher
}

func runMarketplaceBrowse(cmd *cobra.Command, args []string) error {
	deps := &browseDeps{
		stdout:      cmd.OutOrStdout(),
		configPath:  getConfigPath(),
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
		// Fall back to remote fetch
		source, sourceErr := resolveMarketplaceSource(name, deps.configPath)
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

// resolveMarketplaceSource resolves a marketplace name to its source URL.
func resolveMarketplaceSource(name, configPath string) (string, error) {
	if name == "summon-marketplace" {
		return marketplace.OfficialMarketplaceURL, nil
	}

	cfg, err := marketplace.LoadConfig(configPath)
	if err != nil {
		return "", err
	}

	entry := cfg.FindMarketplace(name)
	if entry == nil {
		return "", fmt.Errorf("marketplace %q not found. Use 'summon marketplace add' to register it", name)
	}
	return entry.Source, nil
}

func deriveMarketplaceName(source string) string {
	// Try to extract a clean name from the source URL
	source = strings.TrimSuffix(source, ".git")
	parts := strings.Split(strings.TrimRight(source, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return source
}


