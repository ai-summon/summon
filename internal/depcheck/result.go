package depcheck

// Status represents the satisfaction status of a dependency.
type Status int

const (
	Satisfied             Status = iota // Dependency installed and version matches
	Missing                             // Dependency not installed anywhere
	VersionMismatch                     // Dependency installed but version doesn't match constraint
	UnparseableConstraint               // Constraint string couldn't be parsed
)

// String returns a human-readable label for the status.
func (s Status) String() string {
	switch s {
	case Satisfied:
		return "satisfied"
	case Missing:
		return "missing"
	case VersionMismatch:
		return "version_mismatch"
	case UnparseableConstraint:
		return "unparseable_constraint"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler so status serialises as a string.
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// Result represents the outcome of checking a single dependency.
type Result struct {
	DependencyName   string `json:"dependency_name"`
	Constraint       string `json:"constraint"`
	InstalledVersion string `json:"installed_version"`
	InstalledScope   string `json:"installed_scope"`
	Status           Status `json:"status"`
	Message          string `json:"message"`
}

// PackageCheckResult groups all dependency results for a single package.
type PackageCheckResult struct {
	PackageName  string   `json:"package_name"`
	PackageScope string   `json:"package_scope"`
	Version      string   `json:"version"`
	Results      []Result `json:"results"`
	AllSatisfied bool     `json:"all_satisfied"`
}

// CheckAllResult is the top-level response for `summon check --json`.
type CheckAllResult struct {
	Packages     []PackageCheckResult `json:"packages"`
	AllSatisfied bool                 `json:"all_satisfied"`
	Warnings     []string             `json:"warnings,omitempty"`
}

// ReverseDependent represents a package that depends on a target package.
type ReverseDependent struct {
	PackageName string `json:"package_name"`
	Scope       string `json:"scope"`
	Constraint  string `json:"constraint"`
}
