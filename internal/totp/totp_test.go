package totp

import (
	"strings"
	"testing"
	"time"
)

// rfcSecret is the shared secret of RFC 4226's appendix D, and its base32
// form is what an authenticator app would be given.
const (
	rfcSecret   = "12345678901234567890"
	rfcBase32   = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	rfcBase32Lo = "gezdgnbvgy3tqojqgezdgnbvgy3tqojq"
)

// TestHotpVectors checks the published codes: RFC 4226 appendix D lists the
// six-digit HOTP for counter 0..9, and TOTP with a 30-second step is that
// counter at unix time 30*counter.
func TestHotpVectors(t *testing.T) {
	key, err := Parse(rfcBase32)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	for counter, code := range want {
		at := time.Unix(int64(counter)*30, 0)
		if got := key.Code(at); got != code {
			t.Errorf("counter %d: Code = %q, want %q", counter, got, code)
		}
		if !key.Verify(code, at) {
			t.Errorf("counter %d: Verify(%q) = false", counter, code)
		}
	}
}

func TestParseAcceptsHowSecretsAreCopied(t *testing.T) {
	key, err := Parse(rfcBase32Lo)
	if err != nil {
		t.Fatalf("Parse(lower case): %v", err)
	}
	if key.Code(time.Unix(0, 0)) != "755224" {
		t.Error("the lower-case secret derives a different code")
	}

	// Grouped with spaces or dashes, and with the padding an app may show.
	for _, spelling := range []string{
		"GEZD GNBV GY3T QOJQ GEZD GNBV GY3T QOJQ",
		"GEZD-GNBV-GY3T-QOJQ-GEZD-GNBV-GY3T-QOJQ",
		" GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ= ",
	} {
		grouped, err := Parse(spelling)
		if err != nil {
			t.Fatalf("Parse(%q): %v", spelling, err)
		}
		if grouped.Code(time.Unix(0, 0)) != "755224" {
			t.Errorf("Parse(%q) derives a different code", spelling)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"empty":      "",
		"blank":      "   ",
		"not base32": "not-a-secret!",
		"too short":  "GEZDGNBV", // 5 bytes
	}
	for name, secret := range cases {
		if _, err := Parse(secret); err == nil {
			t.Errorf("%s: Parse(%q) was accepted", name, secret)
		}
	}
}

// A code is accepted in its own step and one step either side, and not
// further: the window is what an unsynchronised clock needs and no more.
func TestVerifyWindow(t *testing.T) {
	key, err := Parse(rfcBase32)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	const step = 30
	current := time.Unix(5*step, 0) // counter 5
	code := key.Code(current)
	if code != "254676" {
		t.Fatalf("Code = %q, want the counter-5 vector", code)
	}

	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"the step itself", current, true},
		{"one step earlier", current.Add(-step * time.Second), true},
		{"one step later", current.Add(step * time.Second), true},
		{"two steps earlier", current.Add(-2 * step * time.Second), false},
		{"two steps later", current.Add(2 * step * time.Second), false},
		{"a minute earlier", current.Add(-time.Minute), false},
	}
	for _, tc := range cases {
		if got := key.Verify(code, tc.at); got != tc.want {
			t.Errorf("%s: Verify = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Match is Verify with the step attached: the caller that has to refuse a code
// it has already accepted is told which step to remember.
func TestMatchReportsTheStep(t *testing.T) {
	key, err := Parse(rfcBase32)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	const step = 30
	at := time.Unix(5*step, 0) // counter 5
	code := key.Code(at)
	if code != "254676" {
		t.Fatalf("Code = %q, want the counter-5 vector", code)
	}

	cases := []struct {
		name      string
		at        time.Time
		wantStep  uint64
		wantMatch bool
	}{
		{"the step itself", at, 5, true},
		{"one step later, so the code is one step old", at.Add(step * time.Second), 5, true},
		{"one step earlier", at.Add(-step * time.Second), 5, true},
		{"two steps later", at.Add(2 * step * time.Second), 0, false},
		{"a minute earlier", at.Add(-time.Minute), 0, false},
	}
	for _, tc := range cases {
		gotStep, got := key.Match(code, tc.at)
		if got != tc.wantMatch || gotStep != tc.wantStep {
			t.Errorf("%s: Match = (%d, %v), want (%d, %v)", tc.name, gotStep, got, tc.wantStep, tc.wantMatch)
		}
		if got := key.Verify(code, tc.at); got != tc.wantMatch {
			t.Errorf("%s: Verify = %v, want %v - Match and Verify disagree", tc.name, got, tc.wantMatch)
		}
	}

	// A code that is not six digits never matches, and there is no step to
	// report for it.
	for _, bad := range []string{"", "25467", "2546761", "25467a"} {
		if gotStep, got := key.Match(bad, at); got || gotStep != 0 {
			t.Errorf("Match(%q) = (%d, %v), want (0, false)", bad, gotStep, got)
		}
	}
}

func TestVerifyRejectsMalformedCodes(t *testing.T) {
	key, err := Parse(rfcBase32)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	at := time.Unix(0, 0)

	for _, code := range []string{"", "75522", "7552241", "75522a", " 755224 ", "７５５２２４"} {
		got := key.Verify(code, at)
		if code == " 755224 " {
			if !got {
				t.Errorf("Verify(%q) = false, want true: surrounding spaces are a copy-paste artefact", code)
			}
			continue
		}
		if got {
			t.Errorf("Verify(%q) = true, want false", code)
		}
	}
}

// The code is six digits, always: a value below 100000 is padded, not
// shortened, because an authenticator app shows six.
func TestCodeIsAlwaysSixDigits(t *testing.T) {
	key, err := Parse(rfcBase32)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for counter := range 200 {
		code := key.Code(time.Unix(int64(counter)*30, 0))
		if len(code) != Digits || strings.Trim(code, "0123456789") != "" {
			t.Fatalf("counter %d: Code = %q, want %d digits", counter, code, Digits)
		}
	}
}
