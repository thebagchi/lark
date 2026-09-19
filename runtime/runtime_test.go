// This file imports only the facade. That is the point of it: if the facade
// stopped re-exporting something, or stopped registering the scheduler's
// builtins, nothing here would compile or pass - which is the whole of what
// phase 11 has to prove.
package runtime_test

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/thebagchi/lark/runtime"
)

const (
	FIXTURE_DIR    = "testdata"
	HOST_FIXTURE   = "host.star"
	BROKEN_FIXTURE = "broken.star"
	EXPECTED_SUM   = 10
	WHY            = "the host should see this"
)

// _Disk is a Loader written the way a host would write one, against the
// interface this package re-exports.
type _Disk struct{}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-19 23:32: initial creation
func (d *_Disk) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the file at name.
//
// Revisions:
//   - 2026-09-19 23:32: initial creation
func (d *_Disk) Load(name string) ([]byte, error) {
	src, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	return src, nil
}

// _Compiled builds the named fixture through the facade or ends the test.
//
// Revisions:
//   - 2026-09-19 23:33: initial creation
func _Compiled(t *testing.T, name string) *runtime.Artifact {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	built, err := runtime.NewCompiler(runtime.WithLoader(&_Disk{})).Compile(name, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return built
}

// TestHost_OneImportIsEnough proves what the facade exists for: a host that
// names no subpackage compiles a script which loads a module, asserts, spawns
// two functions and joins them, and runs it.
//
// If the init that registers the scheduler's builtins were missing, this would
// fail at compile with "undefined: spawn" rather than pass quietly. That is the
// only check standing behind the facade's reason to exist.
//
// Revisions:
//   - 2026-09-19 23:34: initial creation
func TestHost_OneImportIsEnough(t *testing.T) {
	value, err := _Compiled(t, HOST_FIXTURE).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if value.String() != fmt.Sprint(EXPECTED_SUM) {
		t.Fatalf("got %s, want %d", value.String(), EXPECTED_SUM)
	}
}

// TestHost_CanMatchAFailure proves a host can act on what went wrong without
// importing the package that raised it: the sentinel this package re-exports is
// the same value, so errors.Is reaches through.
//
// Revisions:
//   - 2026-09-19 23:35: initial creation
func TestHost_CanMatchAFailure(t *testing.T) {
	_, err := _Compiled(t, BROKEN_FIXTURE).Run(t.Context())
	if !errors.Is(err, runtime.ErrAssert) {
		t.Fatalf("got %v, want ErrAssert", err)
	}

	t.Logf("the host matched it: %v", err)
}

// TestHost_CanInvokeAnyTopLevelFunction proves the entry point is not the only
// thing reachable, and that ENTRY names it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
func TestHost_CanInvokeAnyTopLevelFunction(t *testing.T) {
	built := _Compiled(t, HOST_FIXTURE)

	value, err := built.Invoke(t.Context(), runtime.ENTRY)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if value.String() != fmt.Sprint(EXPECTED_SUM) {
		t.Fatalf("got %s, want %d", value.String(), EXPECTED_SUM)
	}

	_, err = built.Invoke(t.Context(), "left")
	if err != nil {
		t.Fatalf("invoke a non-entry function: %v", err)
	}
}

// TestHost_CanSaveAnArtifact proves the bytes a container format needs are
// reachable from here too.
//
// Revisions:
//   - 2026-09-19 23:37: initial creation
func TestHost_CanSaveAnArtifact(t *testing.T) {
	saved, err := _Compiled(t, HOST_FIXTURE).Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if len(saved) != 2 {
		t.Fatalf("saved %d units, want 2", len(saved))
	}
}
