package syscheck

import (
	"fmt"
	"os/exec"
)

// Requirement represents a system requirement with its check result.
type Requirement struct {
	Name     string
	Optional bool
	Reason   string
	Found    bool
	Path     string
}

// LookPathFunc is a function type for looking up binaries on PATH.
type LookPathFunc func(name string) (string, error)

// DefaultLookPath uses exec.LookPath.
var DefaultLookPath LookPathFunc = exec.LookPath

// Result holds the results of a system requirements check.
type Result struct {
	Requirements []Requirement
	HasRequired  bool // true if any required dependency is missing
}

// Check verifies system requirements against the current PATH.
func Check(requirements []RequirementInput, lookPath LookPathFunc) *Result {
	if lookPath == nil {
		lookPath = DefaultLookPath
	}

	result := &Result{}
	for _, req := range requirements {
		path, err := lookPath(req.Name)
		found := err == nil

		r := Requirement{
			Name:     req.Name,
			Optional: req.Optional,
			Reason:   req.Reason,
			Found:    found,
			Path:     path,
		}
		result.Requirements = append(result.Requirements, r)

		if !found && !req.Optional {
			result.HasRequired = true
		}
	}
	return result
}

// RequirementInput is the input format for system requirements.
type RequirementInput struct {
	Name     string
	Optional bool
	Reason   string
}

// MissingRequired returns the list of missing required dependencies.
func (r *Result) MissingRequired() []Requirement {
	var missing []Requirement
	for _, req := range r.Requirements {
		if !req.Found && !req.Optional {
			missing = append(missing, req)
		}
	}
	return missing
}

// MissingRecommended returns the list of missing recommended (optional) dependencies.
func (r *Result) MissingRecommended() []Requirement {
	var missing []Requirement
	for _, req := range r.Requirements {
		if !req.Found && req.Optional {
			missing = append(missing, req)
		}
	}
	return missing
}

// FormatCheck returns a human-readable string for a single requirement check.
func FormatCheck(req Requirement) string {
	if req.Found {
		return fmt.Sprintf("  ✓ %s (found: %s)", req.Name, req.Path)
	}
	if req.Optional {
		return fmt.Sprintf("  ✗ %s (not found) [recommended: %s]", req.Name, req.Reason)
	}
	return fmt.Sprintf("  ✗ %s (not found) [REQUIRED]", req.Name)
}
