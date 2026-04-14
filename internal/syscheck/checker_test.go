package syscheck

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeLookPath(available map[string]string) LookPathFunc {
	return func(name string) (string, error) {
		if path, ok := available[name]; ok {
			return path, nil
		}
		return "", fmt.Errorf("%s not found", name)
	}
}

func TestCheck_AllFound(t *testing.T) {
	available := map[string]string{
		"python3": "/usr/bin/python3",
		"git":     "/usr/bin/git",
	}
	reqs := []RequirementInput{
		{Name: "python3"},
		{Name: "git"},
	}
	result := Check(reqs, fakeLookPath(available))
	require.Len(t, result.Requirements, 2)
	assert.False(t, result.HasRequired)
	assert.True(t, result.Requirements[0].Found)
	assert.True(t, result.Requirements[1].Found)
}

func TestCheck_RequiredMissing(t *testing.T) {
	available := map[string]string{
		"git": "/usr/bin/git",
	}
	reqs := []RequirementInput{
		{Name: "python3"},
		{Name: "git"},
	}
	result := Check(reqs, fakeLookPath(available))
	assert.True(t, result.HasRequired)
	assert.False(t, result.Requirements[0].Found)
	assert.True(t, result.Requirements[1].Found)
}

func TestCheck_RecommendedMissing(t *testing.T) {
	available := map[string]string{
		"python3": "/usr/bin/python3",
	}
	reqs := []RequirementInput{
		{Name: "python3"},
		{Name: "docker", Optional: true, Reason: "Only needed for containers"},
	}
	result := Check(reqs, fakeLookPath(available))
	assert.False(t, result.HasRequired)
	assert.Len(t, result.MissingRecommended(), 1)
	assert.Equal(t, "docker", result.MissingRecommended()[0].Name)
}

func TestCheck_MixedMissing(t *testing.T) {
	available := map[string]string{}
	reqs := []RequirementInput{
		{Name: "python3"},
		{Name: "docker", Optional: true, Reason: "Containers"},
	}
	result := Check(reqs, fakeLookPath(available))
	assert.True(t, result.HasRequired)
	assert.Len(t, result.MissingRequired(), 1)
	assert.Len(t, result.MissingRecommended(), 1)
}

func TestCheck_Empty(t *testing.T) {
	result := Check(nil, fakeLookPath(nil))
	assert.False(t, result.HasRequired)
	assert.Empty(t, result.Requirements)
}

func TestFormatCheck_Found(t *testing.T) {
	req := Requirement{Name: "python3", Found: true, Path: "/usr/bin/python3"}
	s := FormatCheck(req)
	assert.Contains(t, s, "✓")
	assert.Contains(t, s, "python3")
}

func TestFormatCheck_RequiredMissing(t *testing.T) {
	req := Requirement{Name: "rustc", Found: false, Optional: false}
	s := FormatCheck(req)
	assert.Contains(t, s, "✗")
	assert.Contains(t, s, "REQUIRED")
}

func TestFormatCheck_RecommendedMissing(t *testing.T) {
	req := Requirement{Name: "docker", Found: false, Optional: true, Reason: "Containers"}
	s := FormatCheck(req)
	assert.Contains(t, s, "✗")
	assert.Contains(t, s, "recommended")
}
