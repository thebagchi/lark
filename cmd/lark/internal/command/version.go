package command

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

const (
	// DEVEL is the version a build with no module version reports. The go
	// command stamps it itself for a local build with no VCS information
	// (cmd/go load.setBuildInfo); this command says it too when the stamp is
	// empty or the build information is missing.
	//
	// Revisions:
	//   - 2026-11-04 23:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from cmd/lark (C120b, Q315)
	DEVEL = "(devel)"
)

// GoVersion is the Go version this command reports when the build information
// is missing: the one it was built with, as runtime.Version names it.
//
// Revisions:
//   - 2026-11-04 22:51: initial creation (C67)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _GoVersion to GoVersion, for the tests
//     in tests/ (C120b, Q315, Q314)
func GoVersion() string {
	return runtime.Version()
}

// Version is the line -version prints: "lark <version> <go>", the main
// module's version and the Go version that built it, from info.
//
// The version is what the go command stamped: the tag for go install
// path@v1.2.3, a pseudo-version for go build inside a VCS checkout, and
// "(devel)" for the rest. A nil info, or an empty version, reads as "(devel)".
//
// Revisions:
//   - 2026-11-04 22:51: initial creation (C67)
//   - 2026-11-10 01:37: moved from cmd/lark and renamed _Version to Version, for the tests in
//     tests/ (C120b, Q315, Q314)
func Version(info *debug.BuildInfo) string {
	if info == nil {
		return fmt.Sprintf(
			"lark %s %s",
			DEVEL,
			GoVersion(),
		)
	}

	version := info.Main.Version
	if version == "" {
		version = DEVEL
	}

	return fmt.Sprintf(
		"lark %s %s",
		version,
		info.GoVersion,
	)
}
