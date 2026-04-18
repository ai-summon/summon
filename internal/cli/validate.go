package cli

import (
	"encoding/json"
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
	noColor bool
	stdout  io.Writer
}

var validateCmd = &cobra.Command{
	Use:     "validate",
	Short:   "Validate the summon.yaml in the current directory",
	GroupID: "inspect",
	Long:    `Check the summon.yaml manifest for structural errors and verify that declared system requirements are available.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := &validateDeps{
			runner:  &execRunner{},
			noColor: noColorFlag,
			stdout:  os.Stdout,
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

type validateSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Warnings int `json:"warnings"`
}

// ValidateOutput is the top-level JSON output for validate --json.
type ValidateOutput struct {
	Results []validateResult `json:"results"`
	Summary validateSummary  `json:"summary"`
}

func runValidate(deps *validateDeps) error {
	out := deps.stdout
	s := NewStyles(deps.noColor)

	// 1. Parse summon.yaml
	m, err := manifest.LoadFile("summon.yaml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no summon.yaml found in current directory")
		}
		return fmt.Errorf("✗ summon.yaml syntax error: %w", err)
	}

	var results []validateResult
	errCount := 0
	warnings := 0

	// Syntax check passed
	results = append(results, validateResult{
		Check:   "syntax",
		Status:  "pass",
		Message: "summon.yaml syntax is valid",
	})
	if !validateJSON {
		_, _ = fmt.Fprintf(out, "%s summon.yaml syntax is valid\n", s.StatusIcon("pass"))
	}

	// 2. Check dependencies
	for _, dep := range m.Dependencies {
		r, err := resolveDepName(dep)
		if err != nil || r == "" {
			results = append(results, validateResult{
				Check:   "dependency",
				Status:  "fail",
				Message: fmt.Sprintf("dependency %s — invalid format", dep),
			})
			if !validateJSON {
				_, _ = fmt.Fprintf(out, "%s dependency %s — invalid format\n", s.StatusIcon("fail"), dep)
			}
			errCount++
			continue
		}
		results = append(results, validateResult{
			Check:   "dependency",
			Status:  "pass",
			Message: fmt.Sprintf("dependency %s format valid", dep),
		})
		if !validateJSON {
			_, _ = fmt.Fprintf(out, "%s dependency %s — format valid\n", s.StatusIcon("pass"), dep)
		}
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
			switch {
			case r.Found:
				results = append(results, validateResult{
					Check:   "system_requirement",
					Status:  "pass",
					Message: fmt.Sprintf("system requirement %s — found", sr.Name),
				})
				if !validateJSON {
					_, _ = fmt.Fprintf(out, "%s system requirement %s — found\n", s.StatusIcon("pass"), sr.Name)
				}
			case sr.Optional:
				results = append(results, validateResult{
					Check:   "system_requirement",
					Status:  "warn",
					Message: fmt.Sprintf("system requirement %s — not found on PATH", sr.Name),
				})
				if !validateJSON {
					_, _ = fmt.Fprintf(out, "%s system requirement %s — not found on PATH\n", s.StatusIcon("warn"), sr.Name)
				}
				warnings++
			default:
				results = append(results, validateResult{
					Check:   "system_requirement",
					Status:  "fail",
					Message: fmt.Sprintf("system requirement %s — not found on PATH", sr.Name),
				})
				if !validateJSON {
					_, _ = fmt.Fprintf(out, "%s system requirement %s — not found on PATH\n", s.StatusIcon("fail"), sr.Name)
				}
				errCount++
			}
		}
	}

	// JSON output mode
	if validateJSON {
		passed := len(results) - errCount - warnings
		output := ValidateOutput{
			Results: results,
			Summary: validateSummary{
				Total:    len(results),
				Passed:   passed,
				Failed:   errCount,
				Warnings: warnings,
			},
		}
		if err := json.NewEncoder(out).Encode(output); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		if errCount > 0 {
			return fmt.Errorf("validation failed with %d error(s)", errCount)
		}
		return nil
	}

	_, _ = fmt.Fprintf(out, "\n%d error(s), %d warning(s)\n", errCount, warnings)

	if errCount > 0 {
		return fmt.Errorf("validation failed with %d error(s)", errCount)
	}
	return nil
}
