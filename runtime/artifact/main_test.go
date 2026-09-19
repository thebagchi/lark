package artifact_test

import (
	"os"
	"testing"

	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// TestMain installs the scheduler's builtins before any test compiles a script.
//
// Registration is the facade's job - the only package that imports both the
// registry and the scheduler. The facade now exists and does it, but this test
// binary does not import the facade: it tests the package the facade is built
// on. The registry is package-level state per process, and a test binary is its
// own process, so these tests get an empty registry unless they fill it.
//
// So they install it themselves, which is exactly what a host would do if there
// were no facade. The facade's own tests prove the other half - that a host
// writing no registration at all still gets these names.
//
// Revisions:
//   - 2026-09-19 22:54: initial creation
//   - 2026-09-19 23:38: says why this stays now the facade exists, rather than
//     promising it can go
func TestMain(m *testing.M) {
	plugin.Register(&scheduler.Builtins{})
	plugin.Register(&_Exploding{})

	os.Exit(m.Run())
}
