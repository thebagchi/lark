package artifact_test

import (
	"os"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/core"
)

// TestMain installs the exploding plugin before any test compiles a script.
//
// The runtime's own builtins arrive by importing core, as they do for a host.
// The registry is per process, and a test binary is its own process, so the
// plugin only these tests use is installed here.
//
// Revisions:
//   - 2026-09-19 22:54: initial creation
//   - 2026-09-19 23:38: says why this stays now the facade exists, rather than
//     promising it can go
//   - 2026-09-21 09:46: the builtins are a plugin, imported like any other
func TestMain(m *testing.M) {
	plugin.Register(&_Exploding{})

	os.Exit(m.Run())
}
