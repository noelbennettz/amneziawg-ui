// Package version carries the build's identity: the release, the commit it was
// built from and when. The three are empty in a plain `go build` and filled in
// by the linker, which is what the Makefile and the Dockerfile pass:
//
//	go build -ldflags "-X amneziawg-web-ui/internal/version.Version=… \
//	                   -X amneziawg-web-ui/internal/version.Commit=… \
//	                   -X amneziawg-web-ui/internal/version.BuildTime=…"
//
// Nothing here knows where those values came from, and nothing should: a
// source tarball without a git history is still a build, and "dev" is the
// honest answer for one.
package version

import (
	"runtime"
	"runtime/debug"
)

// The three values the linker overwrites. They are variables and not constants
// for exactly that reason.
var (
	// Version is the release the build calls itself: a git tag when there is
	// one, the date tag of the image, or "dev".
	Version = "dev"
	// Commit is the git commit the build was made from, short form.
	Commit = "unknown"
	// BuildTime is when the build ran, RFC 3339 in UTC.
	BuildTime = "unknown"
)

// Info is what /version answers and what the startup line prints.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	// Go is the toolchain the binary was compiled with: a bug report that
	// names it saves a round trip.
	Go string `json:"go"`
}

// Current returns the build's identity. A binary built without ldflags still
// answers with something true: "dev", and - where the module's version control
// information survived the build - the commit Go recorded itself.
func Current() Info {
	info := Info{Version: Version, Commit: Commit, BuildTime: BuildTime, Go: runtime.Version()}

	if info.Commit == "unknown" {
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range buildInfo.Settings {
				if setting.Key == "vcs.revision" {
					info.Commit = short(setting.Value)
					break
				}
			}
		}
	}

	return info
}

// short is the first eight characters of a revision, the form a person reads.
func short(revision string) string {
	if len(revision) <= 8 {
		return revision
	}
	return revision[:8]
}
