package observe_test

import (
	"os"
	"testing"

	"github.com/thebagchi/lark/runtime/plugin"
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// TestMain installs the scheduler's builtins before any test compiles a script,
// and the flow plugin, which registers itself when imported.
//
// Registration is the facade's job, and this test binary does not import the
// facade: it tests a package the facade is built on. The registry is
// package-level state per process, and a test binary is its own process, so
// these tests get an empty registry unless they fill it.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func TestMain(m *testing.M) {
	plugin.Register(&scheduler.Builtins{})

	os.Exit(m.Run())
}
