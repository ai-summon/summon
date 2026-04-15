package cli

import (
	"fmt"
	"io"
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

func runMarketplaceBrowse(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// For now, show a placeholder — full implementation requires cloning the marketplace repo
	fmt.Fprintf(out, "Browsing marketplace %q...\n", args[0])
	fmt.Fprintln(out, "(Marketplace browsing requires network access to clone the marketplace repository)")
	return nil
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


