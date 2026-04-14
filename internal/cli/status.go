package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ai-summon/summon/internal/installer"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show detected platforms and their capabilities",
	RunE:  runStatus,
}

var statusJSON bool

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(statusCmd)
}

type platformStatus struct {
	Name     string   `json:"name"`
	Detected bool     `json:"detected"`
	Scopes   []string `json:"scopes"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}

	all := platform.AllAdapters(projectDir)
	var statuses []platformStatus
	for _, a := range all {
		var scopes []string
		for _, s := range a.SupportedScopes() {
			scopes = append(scopes, s.String())
		}
		statuses = append(statuses, platformStatus{
			Name:     a.Name(),
			Detected: a.Detect(),
			Scopes:   scopes,
		})
	}

	if statusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(statuses)
	}

	installer.Status("Platform Status", "")
	for _, ps := range statuses {
		mark := "✗"
		if ps.Detected {
			mark = "✓"
		}
		fmt.Fprintf(installer.Stdout, "  %s %s  (scopes: %s)\n",
			mark, ps.Name, joinScopes(ps.Scopes))
	}

	detected := 0
	for _, ps := range statuses {
		if ps.Detected {
			detected++
		}
	}
	if detected == 0 {
		fmt.Fprintln(installer.Stdout, "\n  No AI platform detected. Install Claude Code or GitHub Copilot CLI.")
	}
	return nil
}

func joinScopes(scopes []string) string {
	if len(scopes) == 0 {
		return "none"
	}
	result := scopes[0]
	for _, s := range scopes[1:] {
		result += ", " + s
	}
	return result
}
