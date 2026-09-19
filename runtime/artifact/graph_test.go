package artifact_test

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebagchi/lark/runtime/artifact"
)

const (
	FIXTURE_DIR     = "testdata"
	APP_FIXTURE     = "app.star"
	LIB_FIXTURE     = "lib.star"
	HELPER_FIXTURE  = "helper.star"
	CYCLE_A_FIXTURE = "cycle_a.star"
	EXPECTED_RING   = "cycle_a.star -> cycle_b.star -> cycle_a.star"
	EXPECTED_FETCH  = 1
)

// _Files is a Loader that serves fixtures, counting what it was asked for so a
// test can tell one fetch from two.
type _Files struct {
	dir   string
	calls map[string]int
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
func (f *_Files) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the fixture at name, counting the read.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
func (f *_Files) Load(name string) ([]byte, error) {
	f.calls[name]++

	src, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", name, err)
	}

	return src, nil
}

// _Loader returns a Loader over testdata.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
func _Loader() *_Files {
	return &_Files{
		dir:   FIXTURE_DIR,
		calls: map[string]int{},
	}
}

// _Fixture reads the named script out of testdata.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
func _Fixture(t *testing.T, name string) []byte {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(FIXTURE_DIR, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return src
}

// TestCompile_AllowsAModuleWithoutAnEntryPoint proves a library compiles: what
// a run needs is not what a compile needs.
//
// Revisions:
//   - 2026-09-19 21:45: initial creation
func TestCompile_AllowsAModuleWithoutAnEntryPoint(t *testing.T) {
	_, err := artifact.NewCompiler(artifact.WithLoader(_Loader())).
		Compile(LIB_FIXTURE, _Fixture(t, LIB_FIXTURE))
	if err != nil {
		t.Fatalf("compiling a library: %v", err)
	}
}

// TestCompile_RefusesACycleWithoutRunningAnything proves a cycle is found from
// compiled code, before any top level executes, and costs no fetch of the
// module that closes it.
//
// Revisions:
//   - 2026-09-19 21:45: initial creation
func TestCompile_RefusesACycleWithoutRunningAnything(t *testing.T) {
	loader := _Loader()

	_, err := artifact.NewCompiler(artifact.WithLoader(loader)).
		Compile(CYCLE_A_FIXTURE, _Fixture(t, CYCLE_A_FIXTURE))
	if !errors.Is(err, artifact.ErrCycle) {
		t.Fatalf("compiling a cycle gave %v, want ErrCycle", err)
	}

	if !strings.Contains(err.Error(), EXPECTED_RING) {
		t.Fatalf("refusal does not name the ring: %v", err)
	}

	if loader.calls[CYCLE_A_FIXTURE] != 0 {
		t.Fatalf("the module closing the cycle was fetched %d times", loader.calls[CYCLE_A_FIXTURE])
	}

	t.Logf("refused: %v", err)
}

// TestCompile_FetchesEachModuleOnce proves two routes to one module produce one
// unit rather than two that can drift.
//
// Revisions:
//   - 2026-09-19 21:46: initial creation
func TestCompile_FetchesEachModuleOnce(t *testing.T) {
	loader := _Loader()

	_, err := artifact.NewCompiler(artifact.WithLoader(loader)).
		Compile(APP_FIXTURE, _Fixture(t, APP_FIXTURE))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for _, name := range []string{LIB_FIXTURE, HELPER_FIXTURE} {
		if loader.calls[name] != EXPECTED_FETCH {
			t.Fatalf("%s was fetched %d times, want %d", name, loader.calls[name], EXPECTED_FETCH)
		}
	}
}

// TestCompile_RefusesALoadWithNoLoader proves a host that supplied no way to
// reach modules is told so, rather than handed something that reads like a
// parse failure.
//
// Revisions:
//   - 2026-09-19 21:46: initial creation
func TestCompile_RefusesALoadWithNoLoader(t *testing.T) {
	_, err := artifact.NewCompiler(artifact.WithLoader(nil)).
		Compile(APP_FIXTURE, _Fixture(t, APP_FIXTURE))
	if !errors.Is(err, artifact.ErrNoLoader) {
		t.Fatalf("loading without a loader gave %v, want ErrNoLoader", err)
	}
}
