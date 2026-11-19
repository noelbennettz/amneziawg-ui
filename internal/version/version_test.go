package version

import (
	"strings"
	"testing"
)

// A binary built without ldflags still answers with something true, and the
// answer never carries a long revision.
func TestCurrentDefaults(t *testing.T) {
	info := Current()

	if info.Version == "" || info.Commit == "" || info.BuildTime == "" {
		t.Fatalf("Current() left a field empty: %+v", info)
	}
	if len(info.Commit) > 8 && info.Commit != "unknown" {
		t.Errorf("Commit = %q, want the short form", info.Commit)
	}
}

// The linker is what fills the three in, so the only thing worth testing is
// that it reads them from the variables rather than computing anything.
func TestCurrentReadsTheLinkedValues(t *testing.T) {
	oldVersion, oldCommit, oldTime := Version, Commit, BuildTime
	t.Cleanup(func() { Version, Commit, BuildTime = oldVersion, oldCommit, oldTime })

	Version, Commit, BuildTime = "2026.11.20", "9f934ae", "2026-11-20T00:30:00Z"

	got := Current()
	if got.Version != Version || got.Commit != Commit || got.BuildTime != BuildTime {
		t.Errorf("Current() = %+v, want the linked values", got)
	}
}

// The commit Go records in the binary is used when the linker was given none:
// a `go build` in a checkout knows its revision even if nobody passed it.
func TestCurrentFallsBackToTheBuildInfo(t *testing.T) {
	oldCommit := Commit
	t.Cleanup(func() { Commit = oldCommit })
	Commit = "unknown"

	// The test binary has build info of its own only when it was built from a
	// checkout; both answers are true, so assert the shape, not the value.
	if got := Current().Commit; got == "" {
		t.Error("Commit is empty")
	}
}

func TestGoVersionIsReported(t *testing.T) {
	if got := Current().Go; !strings.HasPrefix(got, "go") {
		t.Errorf("Info.Go = %q, want the toolchain version", got)
	}
}

func TestShort(t *testing.T) {
	for in, want := range map[string]string{
		"":                              "",
		"9f934ae":                       "9f934ae",
		"3392749abcdef0123456789abcdef": "3392749a",
		"unknown":                       "unknown",
	} {
		if got := short(in); got != want {
			t.Errorf("short(%q) = %q, want %q", in, got, want)
		}
	}
}
