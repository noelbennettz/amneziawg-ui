package api

import (
	"strings"
	"testing"
)

// The grammar mirrors newObfChain in amneziawg-go. Both the create form and
// the backend call this one copy, so a value the form accepts cannot come back
// as a rejected request.
func TestValidateCPS(t *testing.T) {
	valid := []string{
		"<b 0xf6ab3267fa>",
		"<b f6ab3267fa>",
		"<t>",
		"<r 20>",
		"<rc 10><rd 5>",
		"<b 0xd100000001><rc 8><t><r 50>",
		"<r 0>",
		"<r 1000>",
		"<d><ds><dz 2>",
		"junk<r 4>between<t>",
	}
	for _, spec := range valid {
		if problem := ValidateCPS("I1", spec); problem != "" {
			t.Errorf("ValidateCPS(%q) = %q, want no problem", spec, problem)
		}
	}

	invalid := []struct {
		spec string
		want string
	}{
		{"<r 20", "missing its closing"},
		{"<>", "empty tag"},
		{"<nope 3>", "unknown tag"},
		{"<b>", "needs a hex sequence"},
		{"<b 0xabc>", "odd number of hex digits"},
		{"<b 0xzz>", "not hexadecimal"},
		{"<r abc>", "needs a byte count"},
		{"<r -1>", "must be in [0, 1000]"},
		{"<rc 1001>", "must be in [0, 1000]"},
		{"nothing here", "no tags"},
	}
	for _, tc := range invalid {
		problem := ValidateCPS("I1", tc.spec)
		if problem == "" {
			t.Errorf("ValidateCPS(%q) = %q, want a problem mentioning %q", tc.spec, problem, tc.want)
			continue
		}
		if !strings.HasPrefix(problem, "I1: ") {
			t.Errorf("ValidateCPS(%q) = %q, want it to name the field", tc.spec, problem)
		}
		if !strings.Contains(problem, tc.want) {
			t.Errorf("ValidateCPS(%q) = %q, want it to mention %q", tc.spec, problem, tc.want)
		}
	}
}
