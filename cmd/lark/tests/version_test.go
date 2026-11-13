package tests

// This file tests the line -version prints, "lark <version> <go>": the main
// module's version when the build stamped one, "(devel)" when it stamped none
// or the build info is missing. A test cannot make the toolchain stamp a tagged
// version into the test binary, so it hands Version a BuildInfo instead.
//
// Revisions:
//   - 2026-11-10 01:37: moved to tests/ from version_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"runtime/debug"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
)

// TestVersion_Line proves the line -version prints is "lark <version> <go>":
// the main module's version when the build stamped one, "(devel)" when it
// stamped none or the build info is missing.
//
// Revisions:
//   - 2026-11-04 22:50: initial creation (C67)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestVersion_Line(t *testing.T) {
	for _, item := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "tagged, as go install path@v1.2.3 builds it",
			info: &debug.BuildInfo{
				GoVersion: "go1.26.10",
				Main: debug.Module{
					Version: "v1.2.3",
				},
			},
			want: "lark v1.2.3 go1.26.10",
		},
		{
			name: "local build, as go build outside a checkout",
			info: &debug.BuildInfo{
				GoVersion: "go1.26.10",
				Main: debug.Module{
					Version: "(devel)",
				},
			},
			want: "lark (devel) go1.26.10",
		},
		{
			name: "no version stamped",
			info: &debug.BuildInfo{
				GoVersion: "go1.26.10",
			},
			want: "lark (devel) go1.26.10",
		},
		{
			name: "no build info",
			info: nil,
			want: "lark (devel) " + command.GoVersion(),
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				got := command.Version(item.info)
				if got != item.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						item.want,
					)
				}
			},
		)
	}
}
