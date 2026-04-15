package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ai-summon/summon/internal/manifest"
	"github.com/ai-summon/summon/internal/platform"
	"github.com/ai-summon/summon/internal/syscheck"
	"github.com/spf13/cobra"
)

var validateJSON bool

type validateDeps struct {
	runner  platform.CommandRunner
	stdout  io.Writer
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the summon.yaml in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := &validateDeps{
			runner: &execRunner{},
			stdout: os.Stdout,
		}
		return runValidate(deps)
	},
}

func init() {
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(validateCmd)
}

type validateResult struct {
	Check   string `json:"check"`
	Status  string `json:"status"` // "pass", "fail", "warn"
	Message string `json:"message"`
}

func runValidate(deps *validateDeps) error {
	out := deps.stdout

	// 1. Parse summon.yaml
	m, err := manifest.LoadFile("summon.yaml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no summon.yaml found in current directory")
		}
		return fmt.Errorf("✗ summon.yaml syntax error: %w", err)
	}

	var results []validateResult
	errors := 0
	warnings := 0

	// Syntax check passed
	results = append(results, validateResult{
		Check:   "syntax",
		Status:  "pass",
		Message: "summon.yaml syntax is valid",
	})
	fmt.Fprintln(out, "✓ summon.yaml syntax is valid")

	// 2. Check dependencies
	for _, dep := range m.Dependencies {
		r, err := resolveDepName(dep)
		if err != nil || r == "" {
			results = append(results, validateResult{
				Check:   "dependency",
				Status:  "fail",
				Message: fmt.Sprintf("dependency %s — invalid format", dep),
			})
			fmt.Fprintf(out, "✗ dependency %s — invalid format\n", dep)
			errors++
			continue
		}
		// For bare names, we'd check the marketplace; for URLs, check reachability
		// For now, validate the format is parseable
		results = append(results, validateResult{
			Check:   "dependency",
			Status:  "pass",
			Message: fmt.Sprintf("dependency %s format valid", dep),
		})
		fmt.Fprintf(out, "✓ dependency %s — format valid\n", dep)
	}

	// 3. Check system requirements
	for _, sr := range m.SystemRequirements {
		req := syscheck.RequirementInput{
			Name:     sr.Name,
			Optional: sr.Optional,
			Reason:   sr.Reason,
		}
		checkResult := syscheck.Check([]syscheck.RequirementInput{req}, nil)
		if len(checkResult.Requirements) > 0 {
			r := checkResult.Requirements[0]
			if r.Found {
				results = append(results, validateResult{
					Check:   "system_requirement",
					Status:  "pass",
					Message: fmt.Sprintf("system requirement %s — found", sr.Name),
				})
				fmt.Fprintf(out, "✓ system requirement %s — found\n", sr.Name)
			} else if sr.Optional {
				results = append(results, validateResult{
					Check:   "system_requirement",
					Status:  "warn",
					Message: fmt.Sprintf("system requirement %s — not found on PATH", sr.Name),
				})
				fmt.Fprintf(out, "⚠ system requirement %s — not found on PATH\n", sr.Name)
				warnings++
			} else {
				results = append(results, validateResult{
					Check:   "system_requirement",
					Status:  "fail",
					Message: fmt.Sprintf("system requirement %s — not found on PATH", sr.Name),
				})
				fmt.Fprintf(out, "✗ system requirement %s — not found on PATH\n", sr.Name)
				errors++
			}
		}
	}

	fmt.Fprintf(out, "\n%d error(s), %d warning(s)\n", errors, warnings)

	if errors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", errors)
	}
	return nil
}
