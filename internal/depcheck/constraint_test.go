package depcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConstraint_Empty(t *testing.T) {
	c, err := ParseConstraint("")
	require.NoError(t, err)
	assert.Nil(t, c) // nil means "any version"
}

func TestParseConstraint_Valid(t *testing.T) {
	cases := []string{
		"^1.2.3",
		"~1.2.3",
		">=1.0.0",
		"<=2.0.0",
		">1.0.0",
		"<2.0.0",
		"!=1.5.0",
		"=1.2.3",
		">=1.0.0, <2.0.0",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			c, err := ParseConstraint(tc)
			require.NoError(t, err)
			assert.NotNil(t, c)
		})
	}
}

func TestParseConstraint_Invalid(t *testing.T) {
	_, err := ParseConstraint("not-a-version")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version constraint")
}

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		constr    string
		satisfied bool
		wantErr   bool
	}{
		// Empty constraint = any version
		{"empty constraint", "1.0.0", "", true, false},

		// Caret: ^1.2.3 = >=1.2.3, <2.0.0
		{"caret match", "1.5.0", "^1.2.3", true, false},
		{"caret too low", "1.1.0", "^1.2.3", false, false},
		{"caret major bump", "2.0.0", "^1.2.3", false, false},

		// Caret zero major: ^0.2.3 = >=0.2.3, <0.3.0
		{"caret zero major match", "0.2.5", "^0.2.3", true, false},
		{"caret zero major too high", "0.3.0", "^0.2.3", false, false},

		// Tilde: ~1.2.3 = >=1.2.3, <1.3.0
		{"tilde match", "1.2.9", "~1.2.3", true, false},
		{"tilde too high", "1.3.0", "~1.2.3", false, false},

		// Comparison operators
		{"gte match", "2.0.0", ">=1.0.0", true, false},
		{"gte exact", "1.0.0", ">=1.0.0", true, false},
		{"gte fail", "0.9.0", ">=1.0.0", false, false},
		{"lte match", "1.0.0", "<=2.0.0", true, false},
		{"lte fail", "3.0.0", "<=2.0.0", false, false},
		{"gt match", "1.0.1", ">1.0.0", true, false},
		{"gt fail", "1.0.0", ">1.0.0", false, false},
		{"lt match", "1.9.9", "<2.0.0", true, false},
		{"lt fail", "2.0.0", "<2.0.0", false, false},
		{"neq match", "1.0.0", "!=1.5.0", true, false},
		{"neq fail", "1.5.0", "!=1.5.0", false, false},

		// Exact
		{"exact match", "1.2.3", "=1.2.3", true, false},
		{"exact fail", "1.2.4", "=1.2.3", false, false},

		// Compound range
		{"compound match", "1.5.0", ">=1.0.0, <2.0.0", true, false},
		{"compound fail", "2.0.0", ">=1.0.0, <2.0.0", false, false},

		// Invalid inputs
		{"invalid version", "notaversion", ">=1.0.0", false, true},
		{"invalid constraint", "1.0.0", "notvalid", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := CheckVersion(tt.version, tt.constr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.satisfied, ok)
			}
		})
	}
}
